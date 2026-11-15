package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/v2/bson"

	"peoplepulse/internal/domain"
	"peoplepulse/internal/httpx"
	"peoplepulse/internal/store"
)

type clearanceView struct {
	*domain.ExitClearance
	Employee       *employeeBrief `json:"employee,omitempty"`
	LastWorkingDay *domain.Date   `json:"lastWorkingDay,omitempty"`
	TotalItems     int            `json:"totalItems"`
	OpenItems      int            `json:"openItems"`
}

type clearanceSummary struct {
	ID         bson.ObjectID `json:"id"`
	Status     string        `json:"status"`
	TotalItems int           `json:"totalItems"`
	OpenItems  int           `json:"openItems"`
	Overridden bool          `json:"overridden"`
}

func summarize(c *domain.ExitClearance) *clearanceSummary {
	return &clearanceSummary{ID: c.ID, Status: c.Status, TotalItems: len(c.Items), OpenItems: c.OpenItems(), Overridden: c.Override != nil}
}

// assigneeResolver returns a function that turns a configured assignee (role,
// user or line manager) into a concrete role or user for one employee. When
// the assignee cannot be resolved, or would be the departing employee, the
// work goes to the HR Manager role and fallback is true.
func (s *Server) assigneeResolver(ctx context.Context, org *domain.Organization, emp *domain.Employee) func(kind string, roleID, userID *bson.ObjectID) (*bson.ObjectID, *bson.ObjectID, bool, error) {
	var hr *bson.ObjectID
	return func(kind string, roleID, userID *bson.ObjectID) (*bson.ObjectID, *bson.ObjectID, bool, error) {
		switch kind {
		case domain.ApproverLineManager:
			if emp.ManagerID != nil {
				u, err := s.store.ActiveUserByEmployeeID(ctx, org.ID, *emp.ManagerID)
				if err == nil {
					return nil, &u.ID, false, nil
				}
				if !errors.Is(err, store.ErrNotFound) {
					return nil, nil, false, err
				}
			}
		case domain.ApproverUser:
			if userID != nil {
				u, err := s.store.UserByID(ctx, org.ID, *userID)
				if err == nil && u.Status == domain.UserStatusActive && (u.EmployeeID == nil || *u.EmployeeID != emp.ID) {
					return nil, userID, false, nil
				}
			}
		case domain.ApproverRole:
			if roleID != nil {
				if _, err := s.store.RoleByID(ctx, org.ID, *roleID); err == nil {
					return roleID, nil, false, nil
				}
			}
		}
		if hr == nil {
			role, err := s.store.RoleByName(ctx, org.ID, "HR Manager")
			if errors.Is(err, store.ErrNotFound) {
				role, err = s.store.LockedRole(ctx, org.ID)
			}
			if err != nil {
				return nil, nil, false, err
			}
			hr = &role.ID
		}
		return hr, nil, true, nil
	}
}

// buildClearance copies the organization's active template onto a checklist
// for one separation. Every item is due on the last working day.
func (s *Server) buildClearance(ctx context.Context, org *domain.Organization, emp *domain.Employee, sep *domain.Separation, now time.Time) (*domain.ExitClearance, error) {
	resolve := s.assigneeResolver(ctx, org, emp)
	c := &domain.ExitClearance{
		ID: bson.NewObjectID(), OrganizationID: org.ID, SeparationID: sep.ID, EmployeeID: emp.ID,
		Items: []domain.ClearanceItem{}, Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	for _, t := range org.Settings.ClearanceTemplate {
		if !t.Active {
			continue
		}
		roleID, userID, fallback, err := resolve(t.AssigneeType, t.RoleID, t.UserID)
		if err != nil {
			return nil, err
		}
		c.Items = append(c.Items, domain.ClearanceItem{
			ItemID: domain.NewItemID(), Category: t.Category, Title: t.Title, AssigneeType: t.AssigneeType,
			RoleID: roleID, UserID: userID, Fallback: fallback, Status: domain.ItemPending, DueDate: sep.LastWorkingDay,
		})
	}
	c.Recompute()
	return c, nil
}

// approvedSideEffects returns what must be saved together with a separation
// that has just become approved: the employee starts serving notice and the
// clearance checklist is generated.
func (s *Server) approvedSideEffects(ctx context.Context, org *domain.Organization, emp *domain.Employee, sep *domain.Separation, now time.Time) (func(ctx context.Context) error, error) {
	clearance, err := s.buildClearance(ctx, org, emp, sep, now)
	if err != nil {
		return nil, err
	}
	return func(ctx context.Context) error {
		if err := s.store.InsertClearance(ctx, clearance); err != nil {
			return err
		}
		return s.store.PatchEmployee(ctx, org.ID, emp.ID, bson.M{"status": domain.EmployeeServingNotice})
	}, nil
}

func hasAssignedItem(p *Principal, c *domain.ExitClearance) bool {
	return slices.ContainsFunc(c.Items, func(i domain.ClearanceItem) bool { return i.AssignedTo(p.User.ID, p.Role.ID) })
}

// loadClearance fetches a checklist the caller may see: HR, the departing
// employee, or anyone with an item assigned to them.
func (s *Server) loadClearance(r *http.Request) (*domain.ExitClearance, error) {
	p := principal(r)
	id, err := pathID(r)
	if err != nil {
		return nil, httpx.NotFound("Clearance")
	}
	c, err := s.store.ClearanceByID(r.Context(), p.User.OrganizationID, id)
	if err != nil {
		return nil, storeErr(err, "Clearance")
	}
	if !p.Can(domain.PermClearanceRead) && !p.Can(domain.PermClearanceManage) && !isOwnEmployee(p, c.EmployeeID) && !hasAssignedItem(p, c) {
		return nil, httpx.NotFound("Clearance")
	}
	return c, nil
}

func (s *Server) writeClearance(w http.ResponseWriter, r *http.Request, status int, c *domain.ExitClearance) {
	view := clearanceView{ExitClearance: c, TotalItems: len(c.Items), OpenItems: c.OpenItems()}
	if e, err := s.store.EmployeeByID(r.Context(), c.OrganizationID, c.EmployeeID); err == nil {
		view.Employee = brief(*e)
	}
	if sep, err := s.store.SeparationByID(r.Context(), c.OrganizationID, c.SeparationID); err == nil {
		view.LastWorkingDay = &sep.LastWorkingDay
	}
	httpx.JSON(w, status, view)
}

func clearanceErr(err error) error {
	switch {
	case errors.Is(err, domain.ErrClearanceCancelled):
		return httpx.Conflict("This clearance was cancelled because the separation was withdrawn.")
	case errors.Is(err, domain.ErrItemNotFound):
		return httpx.NotFound("Clearance item")
	case errors.Is(err, domain.ErrRemarksRequired):
		return httpx.Invalid(map[string]string{"remarks": "is required when an item is blocked or not applicable"})
	}
	return storeErr(err, "Clearance")
}

// requireOpenSeparation refuses changes once the separation is finished.
func (s *Server) requireOpenSeparation(ctx context.Context, c *domain.ExitClearance) error {
	sep, err := s.store.SeparationByID(ctx, c.OrganizationID, c.SeparationID)
	if err != nil {
		return err
	}
	if sep.Status != domain.SeparationApproved {
		return httpx.Conflict("This separation is closed, so its clearance can no longer be changed.")
	}
	return nil
}

func (s *Server) handleListClearances(w http.ResponseWriter, r *http.Request) {
	orgID := principal(r).User.OrganizationID
	status := r.URL.Query().Get("status")
	if status != "" && !slices.Contains([]string{domain.ClearanceInProgress, domain.ClearanceCompleted, domain.ClearanceCancelled}, status) {
		httpx.Fail(w, r, httpx.BadRequest("status must be in_progress, completed or cancelled."))
		return
	}
	page, size := httpx.Page(r)
	list, total, err := s.store.ListClearances(r.Context(), orgID, status, page, size)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out, err := s.clearanceViews(r.Context(), orgID, list)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, 200, map[string]any{"data": out, "page": page, "pageSize": size, "total": total})
}

func (s *Server) clearanceViews(ctx context.Context, orgID bson.ObjectID, list []domain.ExitClearance) ([]clearanceView, error) {
	empIDs, sepIDs := make([]bson.ObjectID, len(list)), make([]bson.ObjectID, len(list))
	for i := range list {
		empIDs[i], sepIDs[i] = list[i].EmployeeID, list[i].SeparationID
	}
	emps, err := s.store.EmployeesByIDs(ctx, orgID, empIDs)
	if err != nil {
		return nil, err
	}
	seps, err := s.store.SeparationsByIDs(ctx, orgID, sepIDs)
	if err != nil {
		return nil, err
	}
	out := make([]clearanceView, len(list))
	for i := range list {
		out[i] = clearanceView{ExitClearance: &list[i], TotalItems: len(list[i].Items), OpenItems: list[i].OpenItems()}
		if e, ok := emps[list[i].EmployeeID]; ok {
			out[i].Employee = brief(e)
		}
		if sep, ok := seps[list[i].SeparationID]; ok {
			day := sep.LastWorkingDay
			out[i].LastWorkingDay = &day
		}
	}
	return out, nil
}

func (s *Server) handleGetClearance(w http.ResponseWriter, r *http.Request) {
	c, err := s.loadClearance(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	s.writeClearance(w, r, 200, c)
}

type myTask struct {
	ClearanceID    bson.ObjectID        `json:"clearanceId"`
	Version        int                  `json:"version"`
	Employee       *employeeBrief       `json:"employee,omitempty"`
	LastWorkingDay *domain.Date         `json:"lastWorkingDay,omitempty"`
	Item           domain.ClearanceItem `json:"item"`
	Overdue        bool                 `json:"overdue"`
}

// handleMyClearanceTasks lists the open items assigned to the caller, earliest due first.
func (s *Server) handleMyClearanceTasks(w http.ResponseWriter, r *http.Request) {
	p := principal(r)
	orgID := p.User.OrganizationID
	list, err := s.store.ClearancesWithOpenItemsFor(r.Context(), orgID, p.User.ID, p.Role.ID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	views, err := s.clearanceViews(r.Context(), orgID, list)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	_, today, err := s.today(r.Context(), orgID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	tasks := []myTask{}
	for _, v := range views {
		for _, item := range v.Items {
			if item.Open() && item.AssignedTo(p.User.ID, p.Role.ID) {
				tasks = append(tasks, myTask{ClearanceID: v.ID, Version: v.Version, Employee: v.Employee,
					LastWorkingDay: v.LastWorkingDay, Item: item, Overdue: item.DueDate.Before(today)})
			}
		}
	}
	slices.SortStableFunc(tasks, func(a, b myTask) int { return a.Item.DueDate.Compare(b.Item.DueDate.Time) })
	httpx.JSON(w, 200, map[string]any{"data": tasks})
}

// handleUpdateClearanceItem lets the assignee or HR set an item's status.
// Only HR can change the due date.
func (s *Server) handleUpdateClearanceItem(w http.ResponseWriter, r *http.Request) {
	p := principal(r)
	var in struct {
		Status  string  `json:"status" validate:"required,oneof=pending cleared not_applicable blocked"`
		Remarks string  `json:"remarks" validate:"max=1000"`
		DueDate *string `json:"dueDate"`
		Version int     `json:"version" validate:"required"`
	}
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	c, err := s.loadClearance(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	itemID := chi.URLParam(r, "itemId")
	item := c.Item(itemID)
	if item == nil {
		httpx.Fail(w, r, httpx.NotFound("Clearance item"))
		return
	}
	manage := p.Can(domain.PermClearanceManage)
	if !manage && !(p.Can(domain.PermClearanceUpdateOwn) && item.AssignedTo(p.User.ID, p.Role.ID)) {
		httpx.Fail(w, r, &httpx.Error{Status: 403, Code: "FORBIDDEN", Message: "This clearance item is assigned to someone else."})
		return
	}
	if err := s.requireOpenSeparation(r.Context(), c); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	before := item.Status
	if in.DueDate != nil {
		if !manage {
			httpx.Fail(w, r, httpx.Forbidden())
			return
		}
		due, err := domain.ParseDate(*in.DueDate)
		if err != nil {
			httpx.Fail(w, r, httpx.Invalid(map[string]string{"dueDate": err.Error()}))
			return
		}
		y, m, d := c.CreatedAt.Date()
		if due.Before(domain.NewDate(y, m, d).AddDays(-1)) {
			httpx.Fail(w, r, httpx.Invalid(map[string]string{"dueDate": "cannot be before the separation was approved"}))
			return
		}
		item.DueDate = due
	}
	if err := c.UpdateItem(itemID, in.Status, strings.TrimSpace(in.Remarks), p.User.ID, time.Now().UTC()); err != nil {
		httpx.Fail(w, r, clearanceErr(err))
		return
	}
	if err := s.store.SaveClearance(r.Context(), c, in.Version); err != nil {
		httpx.Fail(w, r, clearanceErr(err))
		return
	}
	s.audit(r, c.OrganizationID, &p.User.ID, "clearance.item_updated", "clearance", c.ID.Hex(),
		map[string]any{"itemId": itemID, "title": item.Title, "from": before, "to": in.Status, "remarks": in.Remarks})
	c.Version++
	s.writeClearance(w, r, 200, c)
}

// handleAddClearanceItem adds an item that is not in the template to one checklist.
func (s *Server) handleAddClearanceItem(w http.ResponseWriter, r *http.Request) {
	p := principal(r)
	var in struct {
		Category     string  `json:"category" validate:"required,min=2,max=60"`
		Title        string  `json:"title" validate:"required,min=2,max=160"`
		AssigneeType string  `json:"assigneeType" validate:"required,oneof=line_manager role user"`
		RoleID       *string `json:"roleId"`
		UserID       *string `json:"userId"`
		DueDate      *string `json:"dueDate"`
		Version      int     `json:"version" validate:"required"`
	}
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	c, err := s.loadClearance(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := s.requireOpenSeparation(r.Context(), c); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	orgID := c.OrganizationID
	roleID, userID, err := s.parseAssignee(r.Context(), orgID, in.AssigneeType, in.RoleID, in.UserID)
	if err != nil {
		httpx.Fail(w, r, httpx.Invalid(map[string]string{"assigneeType": err.Error()}))
		return
	}
	org, err := s.store.OrganizationByID(r.Context(), orgID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	emp, err := s.store.EmployeeByID(r.Context(), orgID, c.EmployeeID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	sep, err := s.store.SeparationByID(r.Context(), orgID, c.SeparationID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	item := domain.ClearanceItem{ItemID: domain.NewItemID(), Category: strings.TrimSpace(in.Category), Title: strings.TrimSpace(in.Title),
		AssigneeType: in.AssigneeType, Status: domain.ItemPending, DueDate: sep.LastWorkingDay}
	if item.RoleID, item.UserID, item.Fallback, err = s.assigneeResolver(r.Context(), org, emp)(in.AssigneeType, roleID, userID); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if in.DueDate != nil {
		if item.DueDate, err = domain.ParseDate(*in.DueDate); err != nil {
			httpx.Fail(w, r, httpx.Invalid(map[string]string{"dueDate": err.Error()}))
			return
		}
	}
	if err := c.AddItem(item); err != nil {
		httpx.Fail(w, r, clearanceErr(err))
		return
	}
	if err := s.store.SaveClearance(r.Context(), c, in.Version); err != nil {
		httpx.Fail(w, r, clearanceErr(err))
		return
	}
	s.audit(r, orgID, &p.User.ID, "clearance.item_added", "clearance", c.ID.Hex(), map[string]any{"itemId": item.ItemID, "title": item.Title})
	c.Version++
	s.writeClearance(w, r, 201, c)
}

// handleOverrideClearance records HR's decision to let the separation be
// completed while clearance items are still open.
func (s *Server) handleOverrideClearance(w http.ResponseWriter, r *http.Request) {
	p := principal(r)
	var in struct {
		Reason  string `json:"reason" validate:"required,min=5,max=500"`
		Version int    `json:"version" validate:"required"`
	}
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	c, err := s.loadClearance(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if c.Status == domain.ClearanceCancelled {
		httpx.Fail(w, r, clearanceErr(domain.ErrClearanceCancelled))
		return
	}
	if err := s.requireOpenSeparation(r.Context(), c); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	open := c.OpenItems()
	c.Override = &domain.ClearanceOverride{By: p.User.ID, At: time.Now().UTC(), Reason: strings.TrimSpace(in.Reason)}
	if err := s.store.SaveClearance(r.Context(), c, in.Version); err != nil {
		httpx.Fail(w, r, clearanceErr(err))
		return
	}
	s.audit(r, c.OrganizationID, &p.User.ID, "clearance.overridden", "clearance", c.ID.Hex(), map[string]any{"reason": in.Reason, "openItems": open})
	c.Version++
	s.writeClearance(w, r, 200, c)
}

// parseAssignee validates the IDs supplied for a role or user assignee.
func (s *Server) parseAssignee(ctx context.Context, orgID bson.ObjectID, kind string, roleHex, userHex *string) (roleID, userID *bson.ObjectID, err error) {
	switch kind {
	case domain.ApproverLineManager:
		return nil, nil, nil
	case domain.ApproverRole:
		id, err := bson.ObjectIDFromHex(deref(roleHex))
		if err == nil {
			_, err = s.store.RoleByID(ctx, orgID, id)
		}
		if err != nil {
			return nil, nil, errors.New("roleId is not a role in this organization")
		}
		return &id, nil, nil
	case domain.ApproverUser:
		id, err := bson.ObjectIDFromHex(deref(userHex))
		var u *domain.User
		if err == nil {
			u, err = s.store.UserByID(ctx, orgID, id)
		}
		if err != nil || u.Status != domain.UserStatusActive {
			return nil, nil, errors.New("userId is not an active user in this organization")
		}
		return nil, &id, nil
	}
	return nil, nil, errors.New("assigneeType must be line_manager, role or user")
}

// handlePutClearanceTemplate replaces the checklist template. Checklists
// already generated keep the items they were created with.
func (s *Server) handlePutClearanceTemplate(w http.ResponseWriter, r *http.Request) {
	p := principal(r)
	orgID := p.User.OrganizationID
	var in struct {
		ClearanceTemplate []struct {
			Category     string  `json:"category"`
			Title        string  `json:"title"`
			AssigneeType string  `json:"assigneeType"`
			RoleID       *string `json:"roleId"`
			UserID       *string `json:"userId"`
			Active       *bool   `json:"active"`
		} `json:"clearanceTemplate" validate:"required,max=40"`
	}
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	template := []domain.ClearanceTemplateItem{}
	for i, t := range in.ClearanceTemplate {
		bad := func(msg string) {
			httpx.Fail(w, r, httpx.Invalid(map[string]string{"clearanceTemplate": fmt.Sprintf("item %d: %s", i+1, msg)}))
		}
		item := domain.ClearanceTemplateItem{Category: strings.TrimSpace(t.Category), Title: strings.TrimSpace(t.Title),
			AssigneeType: t.AssigneeType, Active: t.Active == nil || *t.Active}
		if n := len(item.Category); n < 2 || n > 60 {
			bad("category must be 2 to 60 characters")
			return
		}
		if n := len(item.Title); n < 2 || n > 160 {
			bad("title must be 2 to 160 characters")
			return
		}
		var err error
		if item.RoleID, item.UserID, err = s.parseAssignee(r.Context(), orgID, t.AssigneeType, t.RoleID, t.UserID); err != nil {
			bad(err.Error())
			return
		}
		template = append(template, item)
	}
	if err := s.store.UpdateOrganization(r.Context(), orgID, bson.M{"settings.clearanceTemplate": template}); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	s.audit(r, orgID, &p.User.ID, "settings.clearance_template.updated", "organization", orgID.Hex(), map[string]any{"items": len(template)})
	httpx.JSON(w, 200, map[string]any{"clearanceTemplate": template})
}
