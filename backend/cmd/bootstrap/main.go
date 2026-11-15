// Command bootstrap creates an organization, its default roles and the first
// Organization Admin. Run it once per organization:
//
//	go run ./cmd/bootstrap -org "Acme Ltd" -email admin@example.com -name "Admin User"
//
// The admin password is read from the BOOTSTRAP_PASSWORD environment variable.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"
	_ "time/tzdata" // time zone data for machines without a system database

	"go.mongodb.org/mongo-driver/v2/bson"

	"peoplepulse/internal/auth"
	"peoplepulse/internal/config"
	"peoplepulse/internal/domain"
	"peoplepulse/internal/store"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "bootstrap failed:", err)
		os.Exit(1)
	}
}

func run() error {
	orgName := flag.String("org", "", "organization name")
	email := flag.String("email", "", "admin email")
	name := flag.String("name", "", "admin full name")
	tz := flag.String("tz", "UTC", "organization IANA time zone")
	flag.Parse()
	password := os.Getenv("BOOTSTRAP_PASSWORD")

	if *orgName == "" || *email == "" || *name == "" {
		return fmt.Errorf("-org, -email and -name are required")
	}
	if _, err := time.LoadLocation(*tz); err != nil {
		return fmt.Errorf("invalid time zone %q", *tz)
	}
	if err := auth.ValidatePassword(password); err != nil {
		return fmt.Errorf("BOOTSTRAP_PASSWORD %w", err)
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	st, err := store.Connect(ctx, cfg.MongoURI, cfg.MongoDB)
	if err != nil {
		return err
	}
	defer st.Close(context.Background())
	if err := st.EnsureIndexes(ctx); err != nil {
		return err
	}

	addr := strings.ToLower(strings.TrimSpace(*email))
	if _, err := st.UserByEmail(ctx, addr); err == nil {
		return fmt.Errorf("a user with email %s already exists", addr)
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	org := &domain.Organization{ID: bson.NewObjectID(), Name: *orgName, TimeZone: *tz, CreatedAt: now, UpdatedAt: now,
		Settings: domain.OrgSettings{EmploymentTypes: domain.DefaultEmploymentTypes()}}
	if err := create(ctx, st, org, addr, *name, hash, now); err != nil {
		// No transaction is used here, so remove anything partially created.
		for _, c := range []string{"users", "roles"} {
			st.DB.Collection(c).DeleteMany(context.Background(), bson.M{"organizationId": org.ID})
		}
		st.DB.Collection("organizations").DeleteOne(context.Background(), bson.M{"_id": org.ID})
		return err
	}
	fmt.Printf("Created organization %q (%s) with admin %s\n", org.Name, org.ID.Hex(), addr)
	return nil
}

func create(ctx context.Context, st *store.Store, org *domain.Organization, email, name, hash string, now time.Time) error {
	if err := st.InsertOrganization(ctx, org); err != nil {
		return err
	}
	var adminRole bson.ObjectID
	for _, r := range domain.DefaultRoles() {
		r.ID, r.OrganizationID, r.CreatedAt, r.UpdatedAt = bson.NewObjectID(), org.ID, now, now
		if err := st.InsertRole(ctx, &r); err != nil {
			return err
		}
		if r.Locked {
			adminRole = r.ID
		}
	}
	return st.InsertUser(ctx, &domain.User{
		ID: bson.NewObjectID(), OrganizationID: org.ID, Email: email, PasswordHash: hash, FullName: name,
		RoleID: adminRole, Status: domain.UserStatusActive, CreatedAt: now, UpdatedAt: now,
	})
}
