package view

import (
	"fmt"
	"html/template"
	"strings"

	"github.com/hitel00000/mold/auth"
	"github.com/hitel00000/mold/resource"
	"github.com/hitel00000/mold/storage"
)

type NavItem struct {
	Name   string
	Table  string
	Active bool
}

type PageData struct {
	Title        string
	NavItems     []NavItem
	CurrentTable string
	Resource     *resource.Resource
	Records      []storage.Record
	Record       storage.Record
	Widgets      []FieldWidget
	Total        int
	Page         int
	PerPage      int
	TotalPages   int
	HasPrev      bool
	HasNext      bool
	PrevPage     int
	NextPage     int
	ErrorMessage string
	ErrorDetails []FieldErrorDetail
	IsEdit       bool
	FlashMessage string
	Session      *auth.Session
	CanCreate    bool
}

type FieldErrorDetail struct {
	Field   string
	Message string
}

const baseLayout = `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{ .Title }} - Mold Runtime</title>
    <style>
        :root {
            --bg-primary: #0f172a;
            --bg-secondary: #1e293b;
            --bg-card: #334155;
            --text-primary: #f8fafc;
            --text-muted: #94a3b8;
            --accent: #38bdf8;
            --accent-hover: #0284c7;
            --danger: #ef4444;
            --danger-hover: #dc2626;
            --border: #475569;
            --radius: 8px;
        }
        * { box-sizing: border-box; margin: 0; padding: 0; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif; }
        body { background-color: var(--bg-primary); color: var(--text-primary); min-height: 100vh; display: flex; flex-direction: column; }
        header { background-color: var(--bg-secondary); border-bottom: 1px solid var(--border); padding: 1rem 2rem; display: flex; align-items: center; justify-content: space-between; }
        .logo { font-size: 1.25rem; font-weight: 700; color: var(--accent); text-decoration: none; display: flex; align-items: center; gap: 0.5rem; }
        nav { display: flex; gap: 0.5rem; align-items: center; }
        nav a { color: var(--text-muted); text-decoration: none; padding: 0.5rem 1rem; border-radius: var(--radius); transition: all 0.2s; font-size: 0.95rem; }
        nav a:hover, nav a.active { color: var(--text-primary); background-color: var(--bg-card); }
        main { flex: 1; max-width: 1200px; width: 100%; margin: 2rem auto; padding: 0 1.5rem; }
        .card { background-color: var(--bg-secondary); border: 1px solid var(--border); border-radius: var(--radius); padding: 1.5rem; margin-bottom: 1.5rem; }
        .flex-between { display: flex; justify-content: space-between; align-items: center; margin-bottom: 1.5rem; }
        .btn { display: inline-flex; align-items: center; justify-content: center; padding: 0.6rem 1.2rem; border-radius: var(--radius); font-weight: 500; font-size: 0.9rem; text-decoration: none; border: none; cursor: pointer; transition: background-color 0.2s; color: white; background-color: var(--accent); }
        .btn:hover { background-color: var(--accent-hover); }
        .btn-secondary { background-color: var(--bg-card); color: var(--text-primary); }
        .btn-secondary:hover { background-color: var(--border); }
        .btn-danger { background-color: var(--danger); }
        .btn-danger:hover { background-color: var(--danger-hover); }
        .btn-sm { padding: 0.35rem 0.75rem; font-size: 0.825rem; }
        table { width: 100%; border-collapse: collapse; text-align: left; }
        th, td { padding: 0.85rem 1rem; border-bottom: 1px solid var(--border); }
        th { background-color: var(--bg-card); color: var(--text-muted); font-weight: 600; font-size: 0.85rem; text-transform: uppercase; letter-spacing: 0.05em; }
        tr:hover { background-color: rgba(255, 255, 255, 0.02); }
        .form-group { margin-bottom: 1.25rem; }
        label { display: block; font-size: 0.9rem; font-weight: 500; color: var(--text-primary); margin-bottom: 0.4rem; }
        input[type="text"], input[type="password"], input[type="number"], input[type="email"], input[type="url"], input[type="datetime-local"], textarea, select {
            width: 100%; padding: 0.65rem 0.85rem; background-color: var(--bg-primary); border: 1px solid var(--border); border-radius: var(--radius); color: var(--text-primary); font-size: 0.95rem;
        }
        input:focus, textarea:focus, select:focus { outline: none; border-color: var(--accent); }
        textarea { min-height: 120px; resize: vertical; }
        .field-desc { font-size: 0.8rem; color: var(--text-muted); margin-top: 0.3rem; }
        .alert { padding: 1rem; border-radius: var(--radius); margin-bottom: 1.5rem; }
        .alert-danger { background-color: rgba(239, 68, 68, 0.15); border: 1px solid var(--danger); color: #fca5a5; }
        .alert-success { background-color: rgba(56, 189, 248, 0.15); border: 1px solid var(--accent); color: #7dd3fc; }
        .pagination { display: flex; justify-content: space-between; align-items: center; margin-top: 1.5rem; }
        .markdown-body { line-height: 1.6; color: #e2e8f0; }
        .markdown-body p { margin-bottom: 1rem; }
        .markdown-body h1, .markdown-body h2, .markdown-body h3 { margin-top: 1.5rem; margin-bottom: 0.75rem; color: var(--text-primary); }
    </style>
</head>
<body>
    <header>
        <a href="/" class="logo"><span>Mold</span> Dashboard</a>
        <nav>
            {{ range .NavItems }}
                <a href="/view/{{ .Table }}" class="{{ if .Active }}active{{ end }}">{{ .Name }}</a>
            {{ end }}
            {{ if .Session }}
                <span style="color: var(--text-muted); margin-left: 1rem; font-size: 0.9rem;">
                    👤 <strong>{{ .Session.Username }}</strong> ({{ .Session.Role }})
                </span>
                <form action="/logout" method="POST" style="display: inline; margin-left: 0.5rem;">
                    <button type="submit" class="btn btn-secondary btn-sm">Logout</button>
                </form>
            {{ else }}
                <a href="/login" class="btn btn-sm" style="margin-left: 1rem;">Login</a>
            {{ end }}
        </nav>
    </header>
    <main>
        {{ if .FlashMessage }}
            <div class="alert alert-success">{{ .FlashMessage }}</div>
        {{ end }}
        {{ if .ErrorMessage }}
            <div class="alert alert-danger">
                <strong>Error:</strong> {{ .ErrorMessage }}
                {{ if .ErrorDetails }}
                    <ul style="margin-top: 0.5rem; margin-left: 1.25rem;">
                        {{ range .ErrorDetails }}
                            <li><strong>{{ .Field }}:</strong> {{ .Message }}</li>
                        {{ end }}
                    </ul>
                {{ end }}
            </div>
        {{ end }}

        {{ template "content" . }}
    </main>
</body>
</html>
`

const listTemplate = `
{{ define "content" }}
<div class="flex-between">
    <div>
        <h2>{{ .Resource.Name }} List</h2>
        <p style="color: var(--text-muted); font-size: 0.9rem; margin-top: 0.2rem;">Total {{ .Total }} records</p>
    </div>
    {{ if .CanCreate }}
        <a href="/view/{{ .CurrentTable }}/create" class="btn">+ Create {{ .Resource.Name }}</a>
    {{ end }}
</div>

<div class="card" style="padding: 0; overflow: hidden;">
    <table>
        <thead>
            <tr>
                <th>ID</th>
                {{ range .Widgets }}
                    <th>{{ .Label }}</th>
                {{ end }}
                <th style="text-align: right;">Actions</th>
            </tr>
        </thead>
        <tbody>
            {{ range $record := .Records }}
                <tr>
                    <td><strong>#{{ index $record "id" }}</strong></td>
                    {{ range $.Widgets }}
                        <td>{{ index $record .Name }}</td>
                    {{ end }}
                    <td style="text-align: right;">
                        {{ if canAccess $.Session $.Resource "read" $record }}
                            <a href="/view/{{ $.CurrentTable }}/{{ index $record "id" }}" class="btn btn-secondary btn-sm">View</a>
                        {{ end }}
                        {{ if canAccess $.Session $.Resource "update" $record }}
                            <a href="/view/{{ $.CurrentTable }}/{{ index $record "id" }}/edit" class="btn btn-secondary btn-sm">Edit</a>
                        {{ end }}
                        {{ if canAccess $.Session $.Resource "delete" $record }}
                            <form action="/view/{{ $.CurrentTable }}/{{ index $record "id" }}/delete" method="POST" style="display: inline;" onsubmit="return confirm('Are you sure you want to delete this {{ $.Resource.Name }}?');">
                                <button type="submit" class="btn btn-danger btn-sm">Delete</button>
                            </form>
                        {{ end }}
                    </td>
                </tr>
            {{ else }}
                <tr>
                    <td colspan="100%" style="text-align: center; color: var(--text-muted); padding: 2rem;">No records found.</td>
                </tr>
            {{ end }}
        </tbody>
    </table>
</div>

<div class="pagination">
    <div>
        Page {{ .Page }} of {{ if eq .TotalPages 0 }}1{{ else }}{{ .TotalPages }}{{ end }}
    </div>
    <div style="display: flex; gap: 0.5rem;">
        {{ if .HasPrev }}
            <a href="/view/{{ .CurrentTable }}?page={{ .PrevPage }}" class="btn btn-secondary btn-sm">&laquo; Prev</a>
        {{ end }}
        {{ if .HasNext }}
            <a href="/view/{{ .CurrentTable }}?page={{ .NextPage }}" class="btn btn-secondary btn-sm">Next &raquo;</a>
        {{ end }}
    </div>
</div>
{{ end }}
`

const detailTemplate = `
{{ define "content" }}
<div class="flex-between">
    <h2>{{ .Resource.Name }} #{{ index .Record "id" }}</h2>
    <div style="display: flex; gap: 0.5rem;">
        <a href="/view/{{ .CurrentTable }}" class="btn btn-secondary">Back to List</a>
        {{ if canAccess .Session .Resource "update" .Record }}
            <a href="/view/{{ .CurrentTable }}/{{ index .Record "id" }}/edit" class="btn">Edit</a>
        {{ end }}
        {{ if canAccess .Session .Resource "delete" .Record }}
            <form action="/view/{{ .CurrentTable }}/{{ index .Record "id" }}/delete" method="POST" style="display: inline;" onsubmit="return confirm('Are you sure you want to delete this {{ .Resource.Name }}?');">
                <button type="submit" class="btn btn-danger">Delete</button>
            </form>
        {{ end }}
    </div>
</div>

<div class="card">
    <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 1.5rem; margin-bottom: 1.5rem;">
        <div>
            <label style="color: var(--text-muted);">ID</label>
            <div style="font-size: 1.1rem; font-weight: 600;">#{{ index .Record "id" }}</div>
        </div>
        {{ if index .Record "created_at" }}
            <div>
                <label style="color: var(--text-muted);">Created At</label>
                <div>{{ index .Record "created_at" }}</div>
            </div>
        {{ end }}
    </div>

    {{ range .Widgets }}
        <div style="margin-bottom: 1.5rem;">
            <label style="color: var(--text-muted);">{{ .Label }}</label>
            {{ $val := index $.Record .Name }}
            {{ if eq .Type "markdown" }}
                <div class="card markdown-body" style="background-color: var(--bg-primary); margin-top: 0.5rem;">
                    {{ renderMarkdown $val }}
                </div>
            {{ else }}
                <div style="font-size: 1.05rem;">{{ if $val }}{{ $val }}{{ else }}<span style="color: var(--text-muted); font-style: italic;">null</span>{{ end }}</div>
            {{ end }}
        </div>
    {{ end }}
</div>
{{ end }}
`

const loginTemplate = `
{{ define "content" }}
<div style="max-width: 450px; margin: 3rem auto;">
    <div class="card">
        <h2 style="margin-bottom: 1.5rem; text-align: center;">Sign In to Mold</h2>
        <form action="/login" method="POST">
            <div class="form-group">
                <label for="username">Username</label>
                <input type="text" id="username" name="username" required autofocus placeholder="Enter your username">
            </div>
            <div class="form-group">
                <label for="password">Password</label>
                <input type="password" id="password" name="password" required placeholder="Enter your password">
            </div>
            <div style="margin-top: 1.5rem;">
                <button type="submit" class="btn" style="width: 100%;">Sign In</button>
            </div>
        </form>
    </div>
</div>
{{ end }}
`

const formTemplate = `
{{ define "content" }}
<div class="flex-between">
    <h2>{{ if .IsEdit }}Edit {{ .Resource.Name }} #{{ index .Record "id" }}{{ else }}Create {{ .Resource.Name }}{{ end }}</h2>
    <a href="/view/{{ .CurrentTable }}" class="btn btn-secondary">Cancel</a>
</div>

<div class="card">
    <form action="{{ if .IsEdit }}/view/{{ .CurrentTable }}/{{ index .Record "id" }}/edit{{ else }}/view/{{ .CurrentTable }}/create{{ end }}" method="POST">
        {{ range .Widgets }}
            {{ $widgetName := .Name }}
            <div class="form-group">
                <label for="{{ .Name }}">{{ .Label }} {{ if .Required }}<span style="color: var(--danger);">*</span>{{ end }}</label>
                {{ $val := .Value }}

                {{ if eq .Kind "textarea" }}
                    <textarea id="{{ .Name }}" name="{{ .Name }}" {{ if .Required }}required{{ end }}>{{ $val }}</textarea>
                {{ else if eq .Kind "select" }}
                    <select id="{{ .Name }}" name="{{ .Name }}" {{ if .Required }}required{{ end }}>
                        <option value="">-- Select {{ .Label }} --</option>
                        {{ range .Options }}
                            <option value="{{ . }}" {{ if eq $val . }}selected{{ end }}>{{ . }}</option>
                        {{ end }}
                    </select>
                {{ else if eq .Kind "checkbox" }}
                    <input type="checkbox" id="{{ .Name }}" name="{{ .Name }}" value="true" {{ if $val }}checked{{ end }}>
                {{ else }}
                    <input type="{{ .Type }}" id="{{ .Name }}" name="{{ .Name }}" value="{{ $val }}" {{ if .Required }}required{{ end }} {{ if .Min }}min="{{ .Min }}"{{ end }} {{ if .Max }}max="{{ .Max }}"{{ end }}>
                {{ end }}

                {{ if .Description }}
                    <div class="field-desc">{{ .Description }}</div>
                {{ end }}

                {{ range $.ErrorDetails }}
                    {{ if eq .Field $widgetName }}
                        <div style="color: var(--danger); font-size: 0.85rem; margin-top: 0.35rem; font-weight: 500;">
                            <strong>Field Error:</strong> {{ .Message }}
                        </div>
                    {{ end }}
                {{ end }}
            </div>
        {{ end }}

        <div style="margin-top: 2rem; display: flex; gap: 1rem;">
            <button type="submit" class="btn">{{ if .IsEdit }}Save Changes{{ else }}Create Record{{ end }}</button>
            <a href="/view/{{ .CurrentTable }}" class="btn btn-secondary">Cancel</a>
        </div>
    </form>
</div>
{{ end }}
`

func createBaseTemplate() (*template.Template, error) {
	funcMap := template.FuncMap{
		"renderMarkdown": func(val any) template.HTML {
			if str, ok := val.(string); ok {
				return RenderMarkdown(str)
			}
			return ""
		},
		"eq": func(a, b any) bool {
			return strings.EqualFold(toString(a), toString(b))
		},
		"canAccess": func(sess *auth.Session, res *resource.Resource, actionStr string, rec storage.Record) bool {
			return auth.Can(sess, res, auth.ActionType(actionStr), rec)
		},
	}

	base := template.New("baseLayout").Funcs(funcMap)
	return base.Parse(baseLayout)
}

func compileTemplates() (listTmpl, detailTmpl, loginTmpl, formTmpl *template.Template, err error) {
	base, err := createBaseTemplate()
	if err != nil {
		return nil, nil, nil, nil, err
	}

	clone := func(tpl string) (*template.Template, error) {
		t, err := base.Clone()
		if err != nil {
			return nil, err
		}
		return t.Parse(tpl)
	}

	listTmpl, err = clone(listTemplate)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	detailTmpl, err = clone(detailTemplate)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	loginTmpl, err = clone(loginTemplate)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	formTmpl, err = clone(formTemplate)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	return listTmpl, detailTmpl, loginTmpl, formTmpl, nil
}

func toString(v any) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprintf("%v", v))
}
