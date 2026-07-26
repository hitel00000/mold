package cloudflare

import (
	"fmt"
	"strings"

	"github.com/hitel00000/mold/plan"
	"github.com/hitel00000/mold/resource"
)

// Output contains the generated Cloudflare Workers files.
type Output struct {
	SchemaSQL      string
	IndexTS        string
	PackageJSON    string
	WranglerConfig string
	Files          map[string]string
}

// Generator generates TypeScript + Hono + Cloudflare D1 code from Mold Resource IR.
type Generator struct{}

// NewGenerator creates a new Cloudflare Workers code generator.
func NewGenerator() *Generator {
	return &Generator{}
}

// Generate consumes a resource.Registry (loaded IR) and produces Cloudflare Workers source code.
func (g *Generator) Generate(reg *resource.Registry) (*Output, error) {
	if reg == nil {
		return nil, fmt.Errorf("registry is nil")
	}

	resources := reg.List()
	if len(resources) == 0 {
		return nil, fmt.Errorf("no resources found in registry")
	}

	schemaSQL := g.generateSchemaSQL(resources)
	indexTS := g.generateIndexTS(resources)
	packageJSON := g.generatePackageJSON()
	wranglerConfig := g.generateWranglerConfig()
	tsConfig := g.generateTSConfig()

	files := map[string]string{
		"schema.sql":     schemaSQL,
		"src/index.ts":   indexTS,
		"package.json":   packageJSON,
		"wrangler.jsonc": wranglerConfig,
		"tsconfig.json":  tsConfig,
	}

	return &Output{
		SchemaSQL:      schemaSQL,
		IndexTS:        indexTS,
		PackageJSON:    packageJSON,
		WranglerConfig: wranglerConfig,
		Files:          files,
	}, nil
}

// generateSchemaSQL builds D1 SQLite DDL for all resources using plan.Build(res).
// Target 1 Migration: Consumes target-agnostic plan.Plan and plan.FieldPlan.
func (g *Generator) generateSchemaSQL(resources []*resource.Resource) string {
	var sb strings.Builder
	sb.WriteString("-- Auto-generated D1 SQLite Schema from Mold Resource IR\n\n")

	for _, res := range resources {
		p := plan.Build(res)

		sb.WriteString(fmt.Sprintf("CREATE TABLE IF NOT EXISTS \"%s\" (\n", p.Table))
		sb.WriteString("    \"id\" INTEGER PRIMARY KEY AUTOINCREMENT")

		// Target 1 Loop over plan.Fields
		for _, f := range p.Fields {
			if f.Deprecated {
				continue
			}

			colType := "TEXT"
			var extra string

			// Target 1 Type Mapping owned by Cloudflare generator
			switch f.Type {
			case resource.TypeString, resource.TypeText, resource.TypeMarkdown, resource.TypeEmail, resource.TypeURL, resource.TypePassword, resource.TypeBlob:
				colType = "TEXT"
			case resource.TypeInt:
				colType = "INTEGER"
			case resource.TypeFloat:
				colType = "REAL"
			case resource.TypeBool:
				colType = "INTEGER"
			case resource.TypeDateTime:
				colType = "TEXT"
			case resource.TypeEnum:
				colType = "TEXT"
				if len(f.Constraints.Values) > 0 {
					strVals := make([]string, len(f.Constraints.Values))
					for i, v := range f.Constraints.Values {
						strVals[i] = fmt.Sprintf("'%s'", v)
					}
					extra += fmt.Sprintf(" CHECK (\"%s\" IN (%s))", f.Name, strings.Join(strVals, ", "))
				}
			}

			if !f.Nullable {
				extra += " NOT NULL"
			}
			if f.Default != nil {
				extra += fmt.Sprintf(" DEFAULT '%v'", f.Default)
			}

			sb.WriteString(fmt.Sprintf(",\n    \"%s\" %s%s", f.Name, colType, extra))
		}

		if p.Timestamps {
			sb.WriteString(",\n    \"created_at\" TEXT NOT NULL")
			sb.WriteString(",\n    \"updated_at\" TEXT NOT NULL")
		}
		if p.SoftDelete {
			sb.WriteString(",\n    \"deleted_at\" TEXT")
		}

		sb.WriteString("\n);\n\n")
	}

	return sb.String()
}

// generateIndexTS builds Hono REST API router and handlers in TypeScript.
// TS Codegen dispatch/loop tracking:
// - Field Loop #2: for _, f := range res.Fields (building TS runtime type checking)
// - Type Dispatch #2: switch f.Type (mapping IR PrimitiveType -> TS type validation logic)
// - Field Loop #3: for _, f := range res.Fields (building DB parameter binding / record formatting)
// - Type Dispatch #3: switch f.Type (mapping IR PrimitiveType -> JS/DB value conversion)
func (g *Generator) generateIndexTS(resources []*resource.Resource) string {
	var sb strings.Builder
	sb.WriteString(`import { Hono } from 'hono';

type Bindings = {
  DB: D1Database;
};

const app = new Hono<{ Bindings: Bindings }>();

// Helper for error envelope
function writeError(c: any, status: number, code: string, message: string) {
  return c.json({ error: { code, message } }, status);
}

interface AuthUser {
  id: any;
  role: string;
}

async function getAuthUser(c: any): Promise<AuthUser | null> {
  const headerUserId = c.req.header('x-user-id');
  if (headerUserId) {
    const role = c.req.header('x-user-role') || 'user';
    return {
      id: isNaN(Number(headerUserId)) ? headerUserId : Number(headerUserId),
      role: role
    };
  }
  const cookieHeader = c.req.header('Cookie') || '';
  const match = cookieHeader.match(/mold_session=([^;]+)/);
  if (match) {
    const token = match[1];
    try {
      const sess = await c.env.DB.prepare('SELECT user_id FROM "_mold_sessions" WHERE id = ? AND expires_at > ?').bind(token, new Date().toISOString()).first<{ user_id: any }>();
      if (sess && sess.user_id != null) {
        const u = await c.env.DB.prepare('SELECT * FROM "users" WHERE id = ?').bind(sess.user_id).first<any>();
        if (u) {
          return { id: u.id, role: u.role || 'user' };
        }
      }
    } catch (e) {
      // Ignore if session table not present
    }
  }
  return null;
}

app.get('/', (c) => c.text('Mold Cloudflare Workers Target API'));
`)

	for _, res := range resources {
		p := plan.Build(res)
		if p == nil {
			continue
		}

		table := p.Table
		endpoint := "/api/" + table

		permCreate := "public"
		permRead := "public"
		permUpdate := "public"
		permDelete := "public"
		ownershipField := ""

		if p.Auth != nil {
			if p.Auth.OwnershipField != "" {
				ownershipField = p.Auth.OwnershipField
			}
			if p.Auth.Permissions.Create != "" {
				permCreate = p.Auth.Permissions.Create
			}
			if p.Auth.Permissions.Read != "" {
				permRead = p.Auth.Permissions.Read
			}
			if p.Auth.Permissions.Update != "" {
				permUpdate = p.Auth.Permissions.Update
			}
			if p.Auth.Permissions.Delete != "" {
				permDelete = p.Auth.Permissions.Delete
			}
		}

		// 1. GET /api/{table} (List)
		sb.WriteString(fmt.Sprintf("\n// LIST /api/%s\n", table))
		sb.WriteString(fmt.Sprintf("app.get('%s', async (c) => {\n", endpoint))
		sb.WriteString("  const authUser = await getAuthUser(c);\n")
		if permRead != "public" {
			sb.WriteString("  if (!authUser) {\n")
			sb.WriteString("    return writeError(c, 401, 'UNAUTHORIZED', 'authentication required');\n")
			sb.WriteString("  }\n")
			if strings.HasPrefix(permRead, "role:") {
				role := strings.TrimPrefix(permRead, "role:")
				sb.WriteString(fmt.Sprintf("  if (authUser.role !== '%s') {\n", role))
				sb.WriteString("    return writeError(c, 403, 'FORBIDDEN', 'forbidden');\n")
				sb.WriteString("  }\n")
			}
		}

		sb.WriteString("  const limit = Math.min(parseInt(c.req.query('limit') || '20', 10), 100);\n")
		sb.WriteString("  const offset = Math.max(parseInt(c.req.query('offset') || '0', 10), 0);\n")

		whereClause := ""
		if p.SoftDelete {
			whereClause = " WHERE \"deleted_at\" IS NULL"
		}

		sb.WriteString(fmt.Sprintf("  const countStmt = await c.env.DB.prepare('SELECT COUNT(*) as total FROM \"%s\"%s').first<{ total: number }>();\n", table, whereClause))
		sb.WriteString("  const total = countStmt ? countStmt.total : 0;\n")
		sb.WriteString(fmt.Sprintf("  const query = 'SELECT * FROM \"%s\"%s ORDER BY id ASC LIMIT ? OFFSET ?';\n", table, whereClause))
		sb.WriteString("  const { results } = await c.env.DB.prepare(query).bind(limit, offset).all();\n")
		sb.WriteString("  return c.json({\n")
		sb.WriteString("    data: results || [],\n")
		sb.WriteString("    meta: { total, limit, offset }\n")
		sb.WriteString("  });\n")
		sb.WriteString("});\n")

		// 2. GET /api/{table}/:id (Detail)
		sb.WriteString(fmt.Sprintf("\n// DETAIL /api/%s/:id\n", table))
		sb.WriteString(fmt.Sprintf("app.get('%s/:id', async (c) => {\n", endpoint))
		sb.WriteString("  const authUser = await getAuthUser(c);\n")
		if permRead != "public" {
			sb.WriteString("  if (!authUser) {\n")
			sb.WriteString("    return writeError(c, 401, 'UNAUTHORIZED', 'authentication required');\n")
			sb.WriteString("  }\n")
		}

		sb.WriteString("  const id = c.req.param('id');\n")
		softCond := ""
		if p.SoftDelete {
			softCond = " AND \"deleted_at\" IS NULL"
		}
		sb.WriteString(fmt.Sprintf("  const record = await c.env.DB.prepare('SELECT * FROM \"%s\" WHERE id = ?%s').bind(id).first();\n", table, softCond))
		sb.WriteString("  if (!record) {\n")
		sb.WriteString("    return writeError(c, 404, 'NOT_FOUND', 'record not found');\n")
		sb.WriteString("  }\n")

		if permRead == "owner" && ownershipField != "" {
			sb.WriteString(fmt.Sprintf("  if (authUser && authUser.role !== 'admin' && (record as any)['%s'] != authUser.id) {\n", ownershipField))
			sb.WriteString("    return writeError(c, 403, 'FORBIDDEN', 'forbidden');\n")
			sb.WriteString("  }\n")
		} else if strings.HasPrefix(permRead, "role:") {
			role := strings.TrimPrefix(permRead, "role:")
			sb.WriteString(fmt.Sprintf("  if (!authUser || authUser.role !== '%s') {\n", role))
			sb.WriteString("    return writeError(c, 403, 'FORBIDDEN', 'forbidden');\n")
			sb.WriteString("  }\n")
		}

		sb.WriteString("  return c.json({ data: record });\n")
		sb.WriteString("});\n")

		// 3. POST /api/{table} (Create)
		sb.WriteString(fmt.Sprintf("\n// CREATE /api/%s\n", table))
		sb.WriteString(fmt.Sprintf("app.post('%s', async (c) => {\n", endpoint))
		sb.WriteString("  const authUser = await getAuthUser(c);\n")
		if permCreate != "public" {
			sb.WriteString("  if (!authUser) {\n")
			sb.WriteString("    return writeError(c, 401, 'UNAUTHORIZED', 'authentication required');\n")
			sb.WriteString("  }\n")
		}
		if strings.HasPrefix(permCreate, "role:") {
			role := strings.TrimPrefix(permCreate, "role:")
			sb.WriteString(fmt.Sprintf("  if (!authUser || authUser.role !== '%s') {\n", role))
			sb.WriteString("    return writeError(c, 403, 'FORBIDDEN', 'forbidden');\n")
			sb.WriteString("  }\n")
		}

		sb.WriteString("  let body: any;\n")
		sb.WriteString("  try {\n")
		sb.WriteString("    body = await c.req.json();\n")
		sb.WriteString("  } catch (e) {\n")
		sb.WriteString("    return writeError(c, 400, 'INVALID_JSON', 'failed to parse json body');\n")
		sb.WriteString("  }\n\n")

		sb.WriteString("  if (body['role'] === 'admin' && (!authUser || authUser.role !== 'admin')) {\n")
		sb.WriteString("    return writeError(c, 403, 'FORBIDDEN', 'cannot grant admin role');\n")
		sb.WriteString("  }\n")

		if ownershipField != "" {
			sb.WriteString(fmt.Sprintf("  if ((body['%s'] === undefined || body['%s'] === null) && authUser) {\n", ownershipField, ownershipField))
			sb.WriteString(fmt.Sprintf("    body['%s'] = authUser.id;\n", ownershipField))
			sb.WriteString("  }\n")
		}

		// Field Loop #2 & Type Dispatch #2 (Validation derived via plan.Plan)
		for _, f := range p.Fields {
			if f.Deprecated {
				continue
			}

			if !f.Nullable && f.Default == nil {
				sb.WriteString(fmt.Sprintf("  if (body['%s'] === undefined || body['%s'] === null) {\n", f.Name, f.Name))
				sb.WriteString(fmt.Sprintf("    return writeError(c, 400, 'VALIDATION_FAILED', 'field %s is required');\n", f.Name))
				sb.WriteString("  }\n")
			}

			// Type Dispatch #2: IR PrimitiveType -> TS type validation
			switch f.Type {
			case resource.TypeString, resource.TypeText, resource.TypeMarkdown, resource.TypeEmail, resource.TypeURL:
				sb.WriteString(fmt.Sprintf("  if (body['%s'] !== undefined && body['%s'] !== null && typeof body['%s'] !== 'string') {\n", f.Name, f.Name, f.Name))
				sb.WriteString(fmt.Sprintf("    return writeError(c, 400, 'VALIDATION_FAILED', 'field %s must be a string');\n", f.Name))
				sb.WriteString("  }\n")
			case resource.TypeInt, resource.TypeFloat:
				sb.WriteString(fmt.Sprintf("  if (body['%s'] !== undefined && body['%s'] !== null && typeof body['%s'] !== 'number') {\n", f.Name, f.Name, f.Name))
				sb.WriteString(fmt.Sprintf("    return writeError(c, 400, 'VALIDATION_FAILED', 'field %s must be a number');\n", f.Name))
				sb.WriteString("  }\n")
			case resource.TypeBool:
				sb.WriteString(fmt.Sprintf("  if (body['%s'] !== undefined && body['%s'] !== null && typeof body['%s'] !== 'boolean') {\n", f.Name, f.Name, f.Name))
				sb.WriteString(fmt.Sprintf("    return writeError(c, 400, 'VALIDATION_FAILED', 'field %s must be a boolean');\n", f.Name))
				sb.WriteString("  }\n")
			}
		}

		cols := []string{}
		vals := []string{}
		bindVars := []string{}

		// Field Loop #3 & Type Dispatch #3 (Parameter Binding derived via plan.Plan)
		for _, f := range p.Fields {
			if f.Deprecated {
				continue
			}
			cols = append(cols, fmt.Sprintf("\"%s\"", f.Name))
			bindVars = append(bindVars, "?")

			// Type Dispatch #3: Value conversion for DB binding
			switch f.Type {
			case resource.TypeBool:
				vals = append(vals, fmt.Sprintf("body['%s'] ? 1 : 0", f.Name))
			default:
				vals = append(vals, fmt.Sprintf("body['%s'] !== undefined ? body['%s'] : null", f.Name, f.Name))
			}
		}

		if p.Timestamps {
			cols = append(cols, "\"created_at\"", "\"updated_at\"")
			bindVars = append(bindVars, "?", "?")
			vals = append(vals, "now", "now")
		}

		sb.WriteString("\n  const now = new Date().toISOString();\n")
		sb.WriteString(fmt.Sprintf("  const insertSql = `INSERT INTO \"%s\" (%s) VALUES (%s) RETURNING *`;\n", table, strings.Join(cols, ", "), strings.Join(bindVars, ", ")))
		sb.WriteString(fmt.Sprintf("  const created = await c.env.DB.prepare(insertSql).bind(%s).first();\n", strings.Join(vals, ", ")))
		sb.WriteString("  return c.json({ data: created }, 201);\n")
		sb.WriteString("});\n")

		// 4. PUT /api/{table}/:id (Update)
		sb.WriteString(fmt.Sprintf("\n// UPDATE /api/%s/:id\n", table))
		sb.WriteString(fmt.Sprintf("app.put('%s/:id', async (c) => {\n", endpoint))
		sb.WriteString("  const authUser = await getAuthUser(c);\n")
		if permUpdate != "public" {
			sb.WriteString("  if (!authUser) {\n")
			sb.WriteString("    return writeError(c, 401, 'UNAUTHORIZED', 'authentication required');\n")
			sb.WriteString("  }\n")
		}

		sb.WriteString("  const id = c.req.param('id');\n")
		softCond = ""
		if p.SoftDelete {
			softCond = " AND \"deleted_at\" IS NULL"
		}
		sb.WriteString(fmt.Sprintf("  const existing = await c.env.DB.prepare('SELECT * FROM \"%s\" WHERE id = ?%s').bind(id).first();\n", table, softCond))
		sb.WriteString("  if (!existing) {\n")
		sb.WriteString("    return writeError(c, 404, 'NOT_FOUND', 'record not found');\n")
		sb.WriteString("  }\n")

		if permUpdate == "owner" && ownershipField != "" {
			sb.WriteString(fmt.Sprintf("  if (authUser && authUser.role !== 'admin' && (existing as any)['%s'] != authUser.id) {\n", ownershipField))
			sb.WriteString("    return writeError(c, 403, 'FORBIDDEN', 'forbidden');\n")
			sb.WriteString("  }\n")
		} else if strings.HasPrefix(permUpdate, "role:") {
			role := strings.TrimPrefix(permUpdate, "role:")
			sb.WriteString(fmt.Sprintf("  if (!authUser || authUser.role !== '%s') {\n", role))
			sb.WriteString("    return writeError(c, 403, 'FORBIDDEN', 'forbidden');\n")
			sb.WriteString("  }\n")
		}

		sb.WriteString("  let body: any;\n")
		sb.WriteString("  try {\n")
		sb.WriteString("    body = await c.req.json();\n")
		sb.WriteString("  } catch (e) {\n")
		sb.WriteString("    return writeError(c, 400, 'INVALID_JSON', 'failed to parse json body');\n")
		sb.WriteString("  }\n\n")

		sb.WriteString("  if (body['role'] !== undefined && body['role'] !== (existing as any)['role'] && body['role'] === 'admin' && (!authUser || authUser.role !== 'admin')) {\n")
		sb.WriteString("    return writeError(c, 403, 'FORBIDDEN', 'cannot grant admin role');\n")
		sb.WriteString("  }\n")

		setClauses := []string{}
		updateVals := []string{}
		for _, f := range p.Fields {
			if f.Deprecated {
				continue
			}
			setClauses = append(setClauses, fmt.Sprintf("\"%s\" = ?", f.Name))
			updateVals = append(updateVals, fmt.Sprintf("body['%s'] !== undefined ? body['%s'] : null", f.Name, f.Name))
		}
		if p.Timestamps {
			setClauses = append(setClauses, "\"updated_at\" = ?")
			updateVals = append(updateVals, "now")
		}
		updateVals = append(updateVals, "id")

		sb.WriteString("  const now = new Date().toISOString();\n")
		sb.WriteString(fmt.Sprintf("  const updateSql = `UPDATE \"%s\" SET %s WHERE id = ?%s RETURNING *`;\n", table, strings.Join(setClauses, ", "), softCond))
		sb.WriteString(fmt.Sprintf("  const updated = await c.env.DB.prepare(updateSql).bind(%s).first();\n", strings.Join(updateVals, ", ")))
		sb.WriteString("  if (!updated) {\n")
		sb.WriteString("    return writeError(c, 404, 'NOT_FOUND', 'record not found');\n")
		sb.WriteString("  }\n")
		sb.WriteString("  return c.json({ data: updated });\n")
		sb.WriteString("});\n")

		// 5. DELETE /api/{table}/:id (Delete)
		sb.WriteString(fmt.Sprintf("\n// DELETE /api/%s/:id\n", table))
		sb.WriteString(fmt.Sprintf("app.delete('%s/:id', async (c) => {\n", endpoint))
		sb.WriteString("  const authUser = await getAuthUser(c);\n")
		if permDelete != "public" {
			sb.WriteString("  if (!authUser) {\n")
			sb.WriteString("    return writeError(c, 401, 'UNAUTHORIZED', 'authentication required');\n")
			sb.WriteString("  }\n")
		}

		sb.WriteString("  const id = c.req.param('id');\n")
		sb.WriteString("  const parsedId = isNaN(Number(id)) ? id : Number(id);\n")

		sb.WriteString(fmt.Sprintf("  const existing = await c.env.DB.prepare('SELECT * FROM \"%s\" WHERE id = ?%s').bind(id).first();\n", table, softCond))
		// Wait, let's fix query: SELECT * FROM "table" WHERE id = ? AND deleted_at IS NULL
		sb.WriteString("  if (!existing) {\n")
		sb.WriteString("    return writeError(c, 404, 'NOT_FOUND', 'record not found');\n")
		sb.WriteString("  }\n")

		if permDelete == "owner" && ownershipField != "" {
			sb.WriteString(fmt.Sprintf("  if (authUser && authUser.role !== 'admin' && (existing as any)['%s'] != authUser.id) {\n", ownershipField))
			sb.WriteString("    return writeError(c, 403, 'FORBIDDEN', 'forbidden');\n")
			sb.WriteString("  }\n")
		} else if strings.HasPrefix(permDelete, "role:") {
			role := strings.TrimPrefix(permDelete, "role:")
			sb.WriteString(fmt.Sprintf("  if (!authUser || authUser.role !== '%s') {\n", role))
			sb.WriteString("    return writeError(c, 403, 'FORBIDDEN', 'forbidden');\n")
			sb.WriteString("  }\n")
		}

		if p.SoftDelete {
			sb.WriteString("  const now = new Date().toISOString();\n")
			sb.WriteString(fmt.Sprintf("  const res = await c.env.DB.prepare('UPDATE \"%s\" SET \"deleted_at\" = ? WHERE id = ? AND \"deleted_at\" IS NULL').bind(now, id).run();\n", table))
			sb.WriteString("  if (!res.meta.changes) {\n")
			sb.WriteString("    return writeError(c, 404, 'NOT_FOUND', 'record not found');\n")
			sb.WriteString("  }\n")
		} else {
			sb.WriteString(fmt.Sprintf("  const res = await c.env.DB.prepare('DELETE FROM \"%s\" WHERE id = ?').bind(id).run();\n", table))
			sb.WriteString("  if (!res.meta.changes) {\n")
			sb.WriteString("    return writeError(c, 404, 'NOT_FOUND', 'record not found');\n")
			sb.WriteString("  }\n")
		}

		sb.WriteString("  return c.json({ data: { deleted: true, id: parsedId } });\n")
		sb.WriteString("});\n")
	}

	sb.WriteString("\nexport default app;\n")
	return sb.String()
}

func (g *Generator) generatePackageJSON() string {
	return `{
  "name": "mold-cloudflare-worker",
  "version": "1.0.0",
  "private": true,
  "scripts": {
    "dev": "wrangler dev",
    "deploy": "wrangler deploy"
  },
  "dependencies": {
    "hono": "^4.7.0"
  },
  "devDependencies": {
    "@cloudflare/workers-types": "^4.20250224.0",
    "typescript": "^5.7.3",
    "wrangler": "^3.111.0"
  }
}
`
}

func (g *Generator) generateWranglerConfig() string {
	return `{
  "$schema": "node_modules/wrangler/config-schema.json",
  "name": "mold-cloudflare-worker",
  "main": "src/index.ts",
  "compatibility_date": "2025-02-24",
  "compatibility_flags": ["nodejs_compat"],
  "d1_databases": [
    {
      "binding": "DB",
      "database_name": "mold-d1",
      "database_id": "local-mold-d1"
    }
  ]
}
`
}

func (g *Generator) generateTSConfig() string {
	return `{
  "compilerOptions": {
    "target": "ES2022",
    "module": "ESNext",
    "moduleResolution": "Bundler",
    "strict": true,
    "lib": ["ES2022"],
    "types": ["@cloudflare/workers-types"]
  },
  "include": ["src/**/*"]
}
`
}

