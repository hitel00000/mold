package view

import (
	"bytes"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"strings"

	"github.com/hitel00000/mold/auth"
	"github.com/hitel00000/mold/resource"
	"github.com/hitel00000/mold/storage"
	"github.com/hitel00000/mold/transport"
)

type ViewHandler struct {
	router     *transport.Router
	overrides  *TemplateOverrides
	listTmpl   *template.Template
	detailTmpl *template.Template
	loginTmpl  *template.Template
	formTmpl   *template.Template
}

func NewViewHandler(router *transport.Router, overrides *TemplateOverrides) (*ViewHandler, error) {
	listTmpl, detailTmpl, loginTmpl, formTmpl, err := compileTemplates()
	if err != nil {
		return nil, fmt.Errorf("failed to compile view templates: %w", err)
	}
	return &ViewHandler{
		router:     router,
		overrides:  overrides,
		listTmpl:   listTmpl,
		detailTmpl: detailTmpl,
		loginTmpl:  loginTmpl,
		formTmpl:   formTmpl,
	}, nil
}

func (vh *ViewHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	path := strings.TrimPrefix(req.URL.Path, "/")
	parts := strings.Split(path, "/")

	sess := vh.extractSession(req)
	reg := vh.router.CurrentRegistry()
	navItems := buildNavItems(reg, "")

	// Handle /login and /logout
	if req.URL.Path == "/login" {
		if req.Method == http.MethodGet {
			vh.renderLogin(w, req, navItems, sess, "")
		} else if req.Method == http.MethodPost {
			vh.handleLoginSubmit(w, req, navItems)
		}
		return
	}

	if req.URL.Path == "/logout" {
		if sess != nil && vh.router.SessionManager() != nil {
			_ = vh.router.SessionManager().DeleteSession(req.Context(), sess.ID)
		}
		auth.ClearSessionCookie(w)
		http.Redirect(w, req, "/login?flash=Logged+out+successfully", http.StatusSeeOther)
		return
	}

	if len(parts) == 0 || parts[0] == "" || parts[0] == "view" && len(parts) == 1 {
		if len(navItems) > 0 {
			http.Redirect(w, req, "/view/"+navItems[0].Table, http.StatusSeeOther)
			return
		}
		vh.renderErrorPage(w, http.StatusNotFound, "No resources registered", nil, sess)
		return
	}

	if parts[0] != "view" || len(parts) < 2 {
		http.NotFound(w, req)
		return
	}

	table := parts[1]
	entry, exists := reg.Lookup(table)
	if !exists {
		vh.renderErrorPage(w, http.StatusNotFound, fmt.Sprintf("Resource table '%s' not found", table), buildNavItems(reg, ""), sess)
		return
	}

	res := entry.Resource
	store := entry.Store
	navItems = buildNavItems(reg, table)

	action := ""
	if len(parts) >= 3 {
		action = parts[2]
	}

	subAction := ""
	if len(parts) >= 4 {
		subAction = parts[3]
	}

	if action == "create" {
		status, allowed, err := auth.Evaluate(sess, res, auth.ActionCreate, nil, nil)
		if !allowed {
			vh.renderErrorPage(w, status, err.Error(), navItems, sess)
			return
		}
		if req.Method == http.MethodGet {
			vh.renderCreateForm(w, res, navItems, table, nil, "", nil, sess)
		} else if req.Method == http.MethodPost {
			vh.handleCreateSubmit(w, req, res, store, navItems, table, sess)
		}
		return
	}

	if action != "" && subAction == "edit" {
		idVal := parseID(action)
		rec, err := store.Get(req.Context(), res, idVal)
		if err != nil {
			vh.renderErrorPage(w, http.StatusNotFound, fmt.Sprintf("Record #%v not found", idVal), navItems, sess)
			return
		}
		status, allowed, err := auth.Evaluate(sess, res, auth.ActionUpdate, rec, nil)
		if !allowed {
			vh.renderErrorPage(w, status, err.Error(), navItems, sess)
			return
		}
		if req.Method == http.MethodGet {
			vh.renderEditForm(w, req, res, store, navItems, table, idVal, nil, "", nil, sess)
		} else if req.Method == http.MethodPost {
			vh.handleEditSubmit(w, req, res, store, navItems, table, idVal, sess)
		}
		return
	}

	if action != "" && subAction == "delete" {
		if req.Method == http.MethodPost {
			idVal := parseID(action)
			rec, err := store.Get(req.Context(), res, idVal)
			if err != nil {
				vh.renderErrorPage(w, http.StatusNotFound, fmt.Sprintf("Record #%v not found", idVal), navItems, sess)
				return
			}
			status, allowed, err := auth.Evaluate(sess, res, auth.ActionDelete, rec, nil)
			if !allowed {
				vh.renderErrorPage(w, status, err.Error(), navItems, sess)
				return
			}
			_ = store.SoftDelete(req.Context(), res, idVal)
			http.Redirect(w, req, fmt.Sprintf("/view/%s?flash=Record+#%v+deleted+successfully", table, idVal), http.StatusSeeOther)
			return
		}
	}

	if action != "" {
		idVal := parseID(action)
		vh.renderDetail(w, req, res, store, navItems, table, idVal, sess)
		return
	}

	vh.renderList(w, req, res, store, navItems, table, sess)
}

func (vh *ViewHandler) renderList(w http.ResponseWriter, req *http.Request, res *resource.Resource, store storage.Store, navItems []NavItem, table string, sess *auth.Session) {
	status, allowed, err := auth.Evaluate(sess, res, auth.ActionRead, nil, nil)
	if !allowed {
		vh.renderErrorPage(w, status, err.Error(), navItems, sess)
		return
	}

	page := 1
	perPage := 20
	if pStr := req.URL.Query().Get("page"); pStr != "" {
		if p, err := strconv.Atoi(pStr); err == nil && p > 0 {
			page = p
		}
	}

	offset := (page - 1) * perPage

	records, err := store.List(req.Context(), res, storage.Query{Limit: perPage, Offset: offset})
	if err != nil {
		vh.renderErrorPage(w, http.StatusInternalServerError, err.Error(), navItems, sess)
		return
	}

	totalRecords, _ := store.List(req.Context(), res, storage.Query{})
	total := len(records)
	if totalRecords != nil {
		total = len(totalRecords)
	}

	sanitizedRecords := transport.SanitizeRecordList(res, records)
	widgets := BuildFormFields(res, nil, false)

	totalPages := (total + perPage - 1) / perPage
	if totalPages == 0 {
		totalPages = 1
	}

	flashMessage := req.URL.Query().Get("flash")

	data := PageData{
		Title:        res.Name + " List",
		NavItems:     navItems,
		CurrentTable: table,
		Resource:     res,
		Records:      sanitizedRecords,
		Widgets:      widgets,
		Total:        total,
		Page:         page,
		PerPage:      perPage,
		TotalPages:   totalPages,
		HasPrev:      page > 1,
		HasNext:      page < totalPages,
		PrevPage:     page - 1,
		NextPage:     page + 1,
		FlashMessage: flashMessage,
		Session:      sess,
		CanCreate:    auth.Can(sess, res, auth.ActionCreate, nil),
	}

	tmpl := vh.listTmpl
	if custom := vh.overrides.Get(table, "list"); custom != nil {
		tmpl = custom
	}
	_ = tmpl.ExecuteTemplate(w, "baseLayout", data)
}

func (vh *ViewHandler) renderDetail(w http.ResponseWriter, req *http.Request, res *resource.Resource, store storage.Store, navItems []NavItem, table string, id any, sess *auth.Session) {
	rec, err := store.Get(req.Context(), res, id)
	if err != nil {
		vh.renderErrorPage(w, http.StatusNotFound, fmt.Sprintf("Record #%v not found", id), navItems, sess)
		return
	}

	status, allowed, authErr := auth.Evaluate(sess, res, auth.ActionRead, rec, nil)
	if !allowed {
		vh.renderErrorPage(w, status, authErr.Error(), navItems, sess)
		return
	}

	sanitized := transport.SanitizeRecord(res, rec)
	widgets := BuildFormFields(res, nil, false)

	data := PageData{
		Title:        fmt.Sprintf("%s #%v", res.Name, id),
		NavItems:     navItems,
		CurrentTable: table,
		Resource:     res,
		Record:       sanitized,
		Widgets:      widgets,
		Session:      sess,
	}

	tmpl := vh.detailTmpl
	if custom := vh.overrides.Get(table, "detail"); custom != nil {
		tmpl = custom
	}
	_ = tmpl.ExecuteTemplate(w, "baseLayout", data)
}

func (vh *ViewHandler) renderCreateForm(w http.ResponseWriter, res *resource.Resource, navItems []NavItem, table string, formValues map[string]any, errMsg string, errDetails []FieldErrorDetail, sess *auth.Session) {
	widgets := BuildFormFields(res, formValues, false)
	data := PageData{
		Title:        "Create " + res.Name,
		NavItems:     navItems,
		CurrentTable: table,
		Resource:     res,
		Widgets:      widgets,
		IsEdit:       false,
		ErrorMessage: errMsg,
		ErrorDetails: errDetails,
		Session:      sess,
	}

	tmpl := vh.formTmpl
	if custom := vh.overrides.Get(table, "form"); custom != nil {
		tmpl = custom
	}

	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "baseLayout", data); err != nil {
		vh.renderErrorPage(w, http.StatusInternalServerError, err.Error(), navItems, sess)
		return
	}

	if errMsg != "" {
		w.WriteHeader(http.StatusBadRequest)
	} else {
		w.WriteHeader(http.StatusOK)
	}
	_, _ = w.Write(buf.Bytes())
}

func (vh *ViewHandler) handleCreateSubmit(w http.ResponseWriter, req *http.Request, res *resource.Resource, store storage.Store, navItems []NavItem, table string, sess *auth.Session) {
	if err := req.ParseForm(); err != nil {
		vh.renderCreateForm(w, res, navItems, table, nil, "Failed to parse form input", nil, sess)
		return
	}

	payload := parseFormPayload(req, res)
	status, allowed, authErr := auth.Evaluate(sess, res, auth.ActionCreate, nil, payload)
	if !allowed {
		vh.renderErrorPage(w, status, authErr.Error(), navItems, sess)
		return
	}

	if sess != nil && res.Auth != nil && res.Auth.OwnershipField != "" {
		if _, exists := payload[res.Auth.OwnershipField]; !exists {
			payload[res.Auth.OwnershipField] = sess.UserID
		}
	}

	created, err := store.Create(req.Context(), res, payload)
	if err != nil {
		errMsg, errDetails := formatValidationError(err)
		vh.renderCreateForm(w, res, navItems, table, payload, errMsg, errDetails, sess)
		return
	}

	createdID := created["id"]
	http.Redirect(w, req, fmt.Sprintf("/view/%s/%v?flash=%s+#%v+created+successfully", table, createdID, res.Name, createdID), http.StatusSeeOther)
}

func (vh *ViewHandler) renderEditForm(w http.ResponseWriter, req *http.Request, res *resource.Resource, store storage.Store, navItems []NavItem, table string, id any, formValues map[string]any, errMsg string, errDetails []FieldErrorDetail, sess *auth.Session) {
	if formValues == nil {
		existingRec, err := store.Get(req.Context(), res, id)
		if err != nil {
			vh.renderErrorPage(w, http.StatusNotFound, fmt.Sprintf("Record #%v not found", id), navItems, sess)
			return
		}
		formValues = existingRec
	}

	sanitizedForm := transport.SanitizeRecord(res, formValues)
	widgets := BuildFormFields(res, sanitizedForm, true)

	data := PageData{
		Title:        fmt.Sprintf("Edit %s #%v", res.Name, id),
		NavItems:     navItems,
		CurrentTable: table,
		Resource:     res,
		Record:       sanitizedForm,
		Widgets:      widgets,
		IsEdit:       true,
		ErrorMessage: errMsg,
		ErrorDetails: errDetails,
		Session:      sess,
	}

	tmpl := vh.formTmpl
	if custom := vh.overrides.Get(table, "form"); custom != nil {
		tmpl = custom
	}

	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "baseLayout", data); err != nil {
		vh.renderErrorPage(w, http.StatusInternalServerError, err.Error(), navItems, sess)
		return
	}

	if errMsg != "" {
		w.WriteHeader(http.StatusBadRequest)
	} else {
		w.WriteHeader(http.StatusOK)
	}
	_, _ = w.Write(buf.Bytes())
}

func (vh *ViewHandler) handleEditSubmit(w http.ResponseWriter, req *http.Request, res *resource.Resource, store storage.Store, navItems []NavItem, table string, id any, sess *auth.Session) {
	if err := req.ParseForm(); err != nil {
		vh.renderEditForm(w, req, res, store, navItems, table, id, nil, "Failed to parse form input", nil, sess)
		return
	}

	payload := parseFormPayload(req, res)
	rec, err := store.Get(req.Context(), res, id)
	if err != nil {
		vh.renderErrorPage(w, http.StatusNotFound, fmt.Sprintf("Record #%v not found", id), navItems, sess)
		return
	}

	status, allowed, authErr := auth.Evaluate(sess, res, auth.ActionUpdate, rec, payload)
	if !allowed {
		vh.renderErrorPage(w, status, authErr.Error(), navItems, sess)
		return
	}

	_, err = store.Update(req.Context(), res, id, payload)
	if err != nil {
		errMsg, errDetails := formatValidationError(err)
		vh.renderEditForm(w, req, res, store, navItems, table, id, payload, errMsg, errDetails, sess)
		return
	}

	http.Redirect(w, req, fmt.Sprintf("/view/%s/%v?flash=%s+#%v+updated+successfully", table, id, res.Name, id), http.StatusSeeOther)
}

func (vh *ViewHandler) renderErrorPage(w http.ResponseWriter, statusCode int, message string, navItems []NavItem, sess *auth.Session) {
	w.WriteHeader(statusCode)
	data := PageData{
		Title:        "Error",
		NavItems:     navItems,
		ErrorMessage: message,
		Session:      sess,
	}
	_ = vh.listTmpl.ExecuteTemplate(w, "baseLayout", data)
}

func (vh *ViewHandler) renderLogin(w http.ResponseWriter, req *http.Request, navItems []NavItem, sess *auth.Session, errMsg string) {
	data := PageData{
		Title:        "Login",
		NavItems:     navItems,
		ErrorMessage: errMsg,
		FlashMessage: req.URL.Query().Get("flash"),
		Session:      sess,
	}
	if errMsg != "" {
		w.WriteHeader(http.StatusBadRequest)
	}
	_ = vh.loginTmpl.ExecuteTemplate(w, "baseLayout", data)
}

func (vh *ViewHandler) handleLoginSubmit(w http.ResponseWriter, req *http.Request, navItems []NavItem) {
	if err := req.ParseForm(); err != nil {
		vh.renderLogin(w, req, navItems, nil, "Failed to parse login input")
		return
	}

	username := req.FormValue("username")
	password := req.FormValue("password")

	reg := vh.router.CurrentRegistry()
	userEntry, exists := reg.Lookup("users")
	if !exists {
		vh.renderLogin(w, req, navItems, nil, "User authentication system not configured")
		return
	}

	records, err := userEntry.Store.List(req.Context(), userEntry.Resource, storage.Query{})
	if err != nil {
		vh.renderLogin(w, req, navItems, nil, "Authentication lookup failed")
		return
	}

	var matchedUser storage.Record
	for _, rec := range records {
		uVal := fmt.Sprintf("%v", rec["username"])
		eVal := fmt.Sprintf("%v", rec["email"])
		if uVal == username || eVal == username {
			matchedUser = rec
			break
		}
	}

	if matchedUser == nil {
		vh.renderLogin(w, req, navItems, nil, "Invalid username or password")
		return
	}

	storedHash := fmt.Sprintf("%v", matchedUser["password"])
	if !auth.CheckPasswordHash(password, storedHash) {
		vh.renderLogin(w, req, navItems, nil, "Invalid username or password")
		return
	}

	roleStr := "user"
	if r, ok := matchedUser["role"].(string); ok && r != "" {
		roleStr = r
	}

	if vh.router.SessionManager() == nil {
		vh.renderLogin(w, req, navItems, nil, "Session manager not configured")
		return
	}

	sess, err := vh.router.SessionManager().CreateSession(req.Context(), matchedUser["id"], username, roleStr)
	if err != nil {
		vh.renderLogin(w, req, navItems, nil, "Failed to create session")
		return
	}

	auth.SetSessionCookie(w, sess.ID)
	http.Redirect(w, req, "/view?flash=Welcome+back+"+username+"!", http.StatusSeeOther)
}

func (vh *ViewHandler) extractSession(req *http.Request) *auth.Session {
	if vh.router == nil || vh.router.SessionManager() == nil {
		return nil
	}
	cookie, err := req.Cookie(auth.SessionCookieName)
	if err != nil || cookie == nil || cookie.Value == "" {
		return nil
	}
	sess, err := vh.router.SessionManager().GetSession(req.Context(), cookie.Value)
	if err != nil {
		return nil
	}
	return sess
}

func buildNavItems(reg *transport.Registry, currentTable string) []NavItem {
	if reg == nil {
		return nil
	}

	var items []NavItem
	// Note: Iterating registry entries
	for table, entry := range reg.Entries() {
		items = append(items, NavItem{
			Name:   entry.Resource.Name,
			Table:  table,
			Active: table == currentTable,
		})
	}
	return items
}

func parseFormPayload(req *http.Request, res *resource.Resource) map[string]any {
	payload := make(map[string]any)

	// Fields
	for _, f := range res.Fields {
		if f.Deprecated {
			continue
		}
		valStr := req.FormValue(f.Name)
		if valStr == "" && f.Nullable {
			continue
		}

		switch f.Type {
		case resource.TypeInt:
			if v, err := strconv.ParseInt(valStr, 10, 64); err == nil {
				payload[f.Name] = v
			}
		case resource.TypeFloat:
			if v, err := strconv.ParseFloat(valStr, 64); err == nil {
				payload[f.Name] = v
			}
		case resource.TypeBool:
			payload[f.Name] = req.FormValue(f.Name) == "true"
		default:
			payload[f.Name] = valStr
		}
	}

	// Relations (belongs_to foreign keys)
	for _, rel := range res.Relations {
		if rel.Kind == resource.KindBelongsTo && rel.ForeignKey != "" {
			valStr := req.FormValue(rel.ForeignKey)
			if valStr != "" {
				if v, err := strconv.ParseInt(valStr, 10, 64); err == nil {
					payload[rel.ForeignKey] = v
				}
			}
		}
	}

	return payload
}

func formatValidationError(err error) (string, []FieldErrorDetail) {
	if err == nil {
		return "", nil
	}

	errStr := err.Error()

	// 1. Foreign Key Constraint Error
	if strings.Contains(errStr, "FOREIGN KEY constraint failed") {
		return "Referenced foreign key target record does not exist", []FieldErrorDetail{
			{Field: "foreign_key", Message: "referenced target record does not exist in target resource"},
		}
	}

	// 2. Field Level Validation Error parsing (e.g. "resource 'Post': field 'title' length 2 is less than min_length 3")
	if strings.Contains(errStr, "field '") {
		parts := strings.Split(errStr, "field '")
		if len(parts) >= 2 {
			subParts := strings.SplitN(parts[1], "' ", 2)
			if len(subParts) == 2 {
				fieldName := subParts[0]
				msg := subParts[1]
				return fmt.Sprintf("Validation failed for field '%s'", fieldName), []FieldErrorDetail{
					{Field: fieldName, Message: msg},
				}
			}
		}
	}

	return errStr, nil
}

func parseID(s string) any {
	if val, err := strconv.ParseInt(s, 10, 64); err == nil {
		return val
	}
	return s
}
