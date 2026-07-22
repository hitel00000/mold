package view

import (
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"strings"

	"github.com/hitel00000/mold/resource"
	"github.com/hitel00000/mold/storage"
	"github.com/hitel00000/mold/transport"
)

type ViewHandler struct {
	router *transport.Router
	tmpl   *template.Template
}

func NewViewHandler(router *transport.Router) (*ViewHandler, error) {
	tmpl, err := compileTemplates()
	if err != nil {
		return nil, fmt.Errorf("failed to compile view templates: %w", err)
	}
	return &ViewHandler{
		router: router,
		tmpl:   tmpl,
	}, nil
}

func (vh *ViewHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	path := strings.TrimPrefix(req.URL.Path, "/")
	parts := strings.Split(path, "/")

	reg := vh.router.CurrentRegistry()
	navItems := buildNavItems(reg, "")

	if len(parts) == 0 || parts[0] == "" || parts[0] == "view" && len(parts) == 1 {
		if len(navItems) > 0 {
			http.Redirect(w, req, "/view/"+navItems[0].Table, http.StatusSeeOther)
			return
		}
		vh.renderErrorPage(w, http.StatusNotFound, "No resources registered", nil)
		return
	}

	if parts[0] != "view" || len(parts) < 2 {
		http.NotFound(w, req)
		return
	}

	table := parts[1]
	entry, exists := reg.Lookup(table)
	if !exists {
		vh.renderErrorPage(w, http.StatusNotFound, fmt.Sprintf("Resource table '%s' not found", table), buildNavItems(reg, ""))
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
		if req.Method == http.MethodGet {
			vh.renderCreateForm(w, res, navItems, table, nil, "", nil)
		} else if req.Method == http.MethodPost {
			vh.handleCreateSubmit(w, req, res, store, navItems, table)
		}
		return
	}

	if action != "" && subAction == "edit" {
		idVal := parseID(action)
		if req.Method == http.MethodGet {
			vh.renderEditForm(w, req, res, store, navItems, table, idVal, nil, "", nil)
		} else if req.Method == http.MethodPost {
			vh.handleEditSubmit(w, req, res, store, navItems, table, idVal)
		}
		return
	}

	if action != "" && subAction == "delete" {
		if req.Method == http.MethodPost {
			idVal := parseID(action)
			_ = store.SoftDelete(req.Context(), res, idVal)
			http.Redirect(w, req, fmt.Sprintf("/view/%s?flash=Record+#%v+deleted+successfully", table, idVal), http.StatusSeeOther)
			return
		}
	}

	if action != "" {
		idVal := parseID(action)
		vh.renderDetail(w, req, res, store, navItems, table, idVal)
		return
	}

	vh.renderList(w, req, res, store, navItems, table)
}

func (vh *ViewHandler) renderList(w http.ResponseWriter, req *http.Request, res *resource.Resource, store storage.Store, navItems []NavItem, table string) {
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
		vh.renderErrorPage(w, http.StatusInternalServerError, err.Error(), navItems)
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
	}

	_ = vh.tmpl.ExecuteTemplate(w, "baseLayout", data)
}

func (vh *ViewHandler) renderDetail(w http.ResponseWriter, req *http.Request, res *resource.Resource, store storage.Store, navItems []NavItem, table string, id any) {
	rec, err := store.Get(req.Context(), res, id)
	if err != nil {
		vh.renderErrorPage(w, http.StatusNotFound, fmt.Sprintf("Record #%v not found", id), navItems)
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
	}

	_ = vh.tmpl.ExecuteTemplate(w, "baseLayout", data)
}

func (vh *ViewHandler) renderCreateForm(w http.ResponseWriter, res *resource.Resource, navItems []NavItem, table string, formValues map[string]any, errMsg string, errDetails []FieldErrorDetail) {
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
	}
	_ = vh.tmpl.ExecuteTemplate(w, "baseLayout", data)
}

func (vh *ViewHandler) handleCreateSubmit(w http.ResponseWriter, req *http.Request, res *resource.Resource, store storage.Store, navItems []NavItem, table string) {
	if err := req.ParseForm(); err != nil {
		vh.renderCreateForm(w, res, navItems, table, nil, "Failed to parse form input", nil)
		return
	}

	payload := parseFormPayload(req, res)
	created, err := store.Create(req.Context(), res, payload)
	if err != nil {
		errMsg, errDetails := formatValidationError(err)
		vh.renderCreateForm(w, res, navItems, table, payload, errMsg, errDetails)
		return
	}

	createdID := created["id"]
	http.Redirect(w, req, fmt.Sprintf("/view/%s/%v?flash=%s+#%v+created+successfully", table, createdID, res.Name, createdID), http.StatusSeeOther)
}

func (vh *ViewHandler) renderEditForm(w http.ResponseWriter, req *http.Request, res *resource.Resource, store storage.Store, navItems []NavItem, table string, id any, formValues map[string]any, errMsg string, errDetails []FieldErrorDetail) {
	if formValues == nil {
		existingRec, err := store.Get(req.Context(), res, id)
		if err != nil {
			vh.renderErrorPage(w, http.StatusNotFound, fmt.Sprintf("Record #%v not found", id), navItems)
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
	}
	_ = vh.tmpl.ExecuteTemplate(w, "baseLayout", data)
}

func (vh *ViewHandler) handleEditSubmit(w http.ResponseWriter, req *http.Request, res *resource.Resource, store storage.Store, navItems []NavItem, table string, id any) {
	if err := req.ParseForm(); err != nil {
		vh.renderEditForm(w, req, res, store, navItems, table, id, nil, "Failed to parse form input", nil)
		return
	}

	payload := parseFormPayload(req, res)
	_, err := store.Update(req.Context(), res, id, payload)
	if err != nil {
		errMsg, errDetails := formatValidationError(err)
		vh.renderEditForm(w, req, res, store, navItems, table, id, payload, errMsg, errDetails)
		return
	}

	http.Redirect(w, req, fmt.Sprintf("/view/%s/%v?flash=%s+#%v+updated+successfully", table, id, res.Name, id), http.StatusSeeOther)
}

func (vh *ViewHandler) renderErrorPage(w http.ResponseWriter, statusCode int, message string, navItems []NavItem) {
	w.WriteHeader(statusCode)
	data := PageData{
		Title:        "Error",
		NavItems:     navItems,
		ErrorMessage: message,
	}
	_ = vh.tmpl.ExecuteTemplate(w, "baseLayout", data)
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
	return err.Error(), nil
}

func parseID(s string) any {
	if val, err := strconv.ParseInt(s, 10, 64); err == nil {
		return val
	}
	return s
}
