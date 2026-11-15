package domain

import (
	"encoding/json"
	"errors"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestDateJSONAndBSONRoundTrip(t *testing.T) {
	type doc struct {
		Join Date  `bson:"join" json:"join"`
		End  *Date `bson:"end,omitempty" json:"end,omitempty"`
	}
	end := NewDate(2026, 3, 31)
	in := doc{Join: NewDate(2024, 2, 29), End: &end}

	j, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	if string(j) != `{"join":"2024-02-29","end":"2026-03-31"}` {
		t.Errorf("unexpected JSON: %s", j)
	}
	var fromJSON doc
	if err := json.Unmarshal(j, &fromJSON); err != nil || !fromJSON.Join.Equal(in.Join) || !fromJSON.End.Equal(end) {
		t.Errorf("JSON round trip failed: %+v, %v", fromJSON, err)
	}

	b, err := bson.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	if typ := bson.Raw(b).Lookup("join").Type; typ != bson.TypeDateTime {
		t.Errorf("join stored as %v, want a BSON datetime", typ)
	}
	var fromBSON doc
	if err := bson.Unmarshal(b, &fromBSON); err != nil || !fromBSON.Join.Equal(in.Join) || fromBSON.End == nil || !fromBSON.End.Equal(end) {
		t.Errorf("BSON round trip failed: %+v, %v", fromBSON, err)
	}

	noEnd, _ := bson.Marshal(doc{Join: in.Join})
	if _, err := bson.Raw(noEnd).LookupErr("end"); err == nil {
		t.Error("nil end date should be omitted from BSON")
	}
	if _, err := ParseDate("31/03/2026"); err == nil {
		t.Error("non-ISO date was accepted")
	}
}

func TestApplyTransfer(t *testing.T) {
	a, b := bson.NewObjectID(), bson.NewObjectID()
	history := []DepartmentAssignment{{DepartmentID: a, From: NewDate(2025, 1, 10)}}

	got, err := ApplyTransfer(history, b, NewDate(2025, 6, 1))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].To == nil || got[0].To.String() != "2025-05-31" || got[1].DepartmentID != b || got[1].To != nil {
		t.Errorf("unexpected history: %+v", got)
	}
	if history[0].To != nil {
		t.Error("ApplyTransfer modified the caller's history")
	}

	if _, err := ApplyTransfer(history, a, NewDate(2025, 6, 1)); !errors.Is(err, ErrSameDepartment) {
		t.Errorf("same department: got %v", err)
	}
	// On or before the start of the current assignment would create an empty or negative period.
	for _, d := range []Date{NewDate(2025, 1, 10), NewDate(2024, 12, 1)} {
		if _, err := ApplyTransfer(history, b, d); !errors.Is(err, ErrTransferTooEarly) {
			t.Errorf("effective %s: got %v", d, err)
		}
	}
}

func TestDefaultRolesUseOnlyCataloguePermissions(t *testing.T) {
	admins := 0
	for _, r := range DefaultRoles() {
		for _, p := range r.Permissions {
			if !IsValidPermission(p) {
				t.Errorf("role %q has unknown permission %q", r.Name, p)
			}
		}
		if r.Locked {
			admins++
		}
	}
	if admins != 1 {
		t.Errorf("want exactly one locked admin role, got %d", admins)
	}
}
