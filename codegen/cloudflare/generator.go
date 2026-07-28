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

		if p.SoftDelete {
			for _, f := range p.Fields {
				if f.Constraints.Unique {
					idxName := fmt.Sprintf("idx_%s_%s_unique", p.Table, f.Name)
					sb.WriteString(fmt.Sprintf("CREATE UNIQUE INDEX IF NOT EXISTS \"%s\" ON \"%s\"(\"%s\") WHERE \"deleted_at\" IS NULL;\n", idxName, p.Table, f.Name))
				}
			}
		}

		for _, group := range p.UniqueTogether {
			if len(group) == 0 {
				continue
			}
			quotedCols := make([]string, 0, len(group))
			for _, col := range group {
				quotedCols = append(quotedCols, fmt.Sprintf("\"%s\"", col))
			}
			idxName := fmt.Sprintf("idx_%s_unique_%s", p.Table, strings.Join(group, "_"))
			if p.SoftDelete {
				sb.WriteString(fmt.Sprintf("CREATE UNIQUE INDEX IF NOT EXISTS \"%s\" ON \"%s\"(%s) WHERE \"deleted_at\" IS NULL;\n", idxName, p.Table, strings.Join(quotedCols, ", ")))
			} else {
				sb.WriteString(fmt.Sprintf("CREATE UNIQUE INDEX IF NOT EXISTS \"%s\" ON \"%s\"(%s);\n", idxName, p.Table, strings.Join(quotedCols, ", ")))
			}
		}
	}

	sb.WriteString(`CREATE TABLE IF NOT EXISTS "_mold_sessions" (
    "id" TEXT PRIMARY KEY,
    "user_id" INTEGER NOT NULL,
    "created_at" TEXT NOT NULL,
    "expires_at" TEXT NOT NULL
);

`)

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
  BUCKET: R2Bucket;
};

const app = new Hono<{ Bindings: Bindings }>();

// Helper for error envelope
function writeError(c: any, status: number, code: string, message: string, details?: any) {
  const err: any = { code, message };
  if (details !== undefined && details !== null) {
    Object.assign(err, details);
  }
  return c.json({ error: err }, status);
}

async function hashPassword(plain: string): Promise<string> {
  const encoder = new TextEncoder();
  const salt = crypto.getRandomValues(new Uint8Array(16));
  const keyMaterial = await crypto.subtle.importKey('raw', encoder.encode(plain), 'PBKDF2', false, ['deriveBits']);
  const iterations = 100000;
  const derivedBits = await crypto.subtle.deriveBits({ name: 'PBKDF2', salt, iterations, hash: 'SHA-256' }, keyMaterial, 256);
  const saltHex = Array.from(salt).map(b => b.toString(16).padStart(2, '0')).join('');
  const hashHex = Array.from(new Uint8Array(derivedBits)).map(b => b.toString(16).padStart(2, '0')).join('');
  return '$pbkdf2$' + iterations + '$' + saltHex + '$' + hashHex;
}

async function verifyPassword(plain: string, storedHash: string): Promise<boolean> {
  if (!storedHash || !storedHash.startsWith('$pbkdf2$')) {
    return false;
  }
  const parts = storedHash.split('$');
  if (parts.length !== 5) return false;
  const iterations = parseInt(parts[2], 10);
  const saltHex = parts[3];
  const expectedHashHex = parts[4];

  const saltBytes = new Uint8Array(saltHex.match(/.{1,2}/g)?.map(b => parseInt(b, 16)) || []);
  const encoder = new TextEncoder();
  const keyMaterial = await crypto.subtle.importKey('raw', encoder.encode(plain), 'PBKDF2', false, ['deriveBits']);
  const derivedBits = await crypto.subtle.deriveBits({ name: 'PBKDF2', salt: saltBytes, iterations, hash: 'SHA-256' }, keyMaterial, 256);
  const derivedHashHex = Array.from(new Uint8Array(derivedBits)).map(b => b.toString(16).padStart(2, '0')).join('');
  return derivedHashHex === expectedHashHex;
}

function sanitizeRecord(record: any, passwordFields: string[]) {
  if (!record) return record;
  const copy = { ...record };
  for (const pf of passwordFields) {
    delete copy[pf];
  }
  return copy;
}

function escapeHTML(str: any): string {
  if (str === null || str === undefined) return '';
  return String(str)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}

function sanitizeHTML(html: string): string {
  if (!html) return '';
  return String(html)
    .replace(/<script\b[^<]*(?:(?!<\/script>)<[^<]*)*<\/script>/gi, '')
    .replace(/on\w+\s*=\s*"[^"]*"/gi, '')
    .replace(/on\w+\s*=\s*'[^']*'/gi, '')
    .replace(/on\w+\s*=\s*[^\s>]+/gi, '')
    .replace(/javascript:[^\s"']*/gi, '#');
}

interface AuthUser {
  id: any;
  role: string;
}

// Resolves authenticated user strictly from session cookie token stored in D1 (_mold_sessions).
// Security Note: Unverified HTTP headers like x-user-id / x-user-role are explicitly rejected
// to prevent client-side header spoofing attacks.
async function getAuthUser(c: any): Promise<AuthUser | null> {
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

		var pwdFields []string
		for _, f := range p.Fields {
			if f.Type == resource.TypePassword {
				pwdFields = append(pwdFields, f.Name)
			}
		}
		pwdFieldsJS := "[]"
		if len(pwdFields) > 0 {
			quoted := make([]string, len(pwdFields))
			for i, name := range pwdFields {
				quoted[i] = fmt.Sprintf("'%s'", name)
			}
			pwdFieldsJS = "[" + strings.Join(quoted, ", ") + "]"
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
		sb.WriteString(fmt.Sprintf("  const sanitized = (results || []).map((r: any) => sanitizeRecord(r, %s));\n", pwdFieldsJS))
		sb.WriteString("  return c.json({\n")
		sb.WriteString("    data: sanitized,\n")
		sb.WriteString("    meta: { total, limit, offset }\n")
		sb.WriteString("  });\n")
		sb.WriteString("});\n")

		// 2. GET /api/{table}/:id (Detail)
		sb.WriteString(fmt.Sprintf("\n// DETAIL /api/%s/:id\n", table))
		sb.WriteString(fmt.Sprintf("app.get('%s/:id', async (c) => {\n", endpoint))
		sb.WriteString("  const authUser = await getAuthUser(c);\n")
		if permRead != "public" && !(permRead == "owner" && ownershipField != "") {
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
			sb.WriteString(fmt.Sprintf("  const ownerVal = (record as any)['%s'];\n", ownershipField))
			sb.WriteString("  if (ownerVal !== null && ownerVal !== undefined) {\n")
			sb.WriteString("    if (!authUser) {\n")
			sb.WriteString("      return writeError(c, 401, 'UNAUTHORIZED', 'authentication required');\n")
			sb.WriteString("    }\n")
			sb.WriteString("    if (authUser.role !== 'admin' && ownerVal != authUser.id) {\n")
			sb.WriteString("      return writeError(c, 403, 'FORBIDDEN', 'forbidden');\n")
			sb.WriteString("    }\n")
			sb.WriteString("  }\n")
		} else if strings.HasPrefix(permRead, "role:") {
			role := strings.TrimPrefix(permRead, "role:")
			sb.WriteString(fmt.Sprintf("  if (!authUser || (authUser.role !== '%s' && authUser.role !== 'admin')) {\n", role))
			sb.WriteString("    return writeError(c, 403, 'FORBIDDEN', 'forbidden');\n")
			sb.WriteString("  }\n")
		}

		sb.WriteString(fmt.Sprintf("  return c.json({ data: sanitizeRecord(record, %s) });\n", pwdFieldsJS))
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

		sb.WriteString("  let body: any = {};\n")
		sb.WriteString("  let formData: FormData | null = null;\n")
		sb.WriteString("  const rawHeader = c.req.header('content-type') || c.req.header('Content-Type') || (c.req.raw && c.req.raw.headers ? c.req.raw.headers.get('content-type') : '') || '';\n")
		sb.WriteString("  const contentType = String(rawHeader).toLowerCase();\n")
		sb.WriteString("  if (contentType.includes('multipart/form-data')) {\n")
		sb.WriteString("    try {\n")
		sb.WriteString("      formData = await c.req.formData();\n")
		sb.WriteString("      formData.forEach((val, key) => {\n")
		sb.WriteString("        if (typeof val === 'string') { body[key] = (val !== '' && !isNaN(Number(val))) ? Number(val) : val; }\n")
		sb.WriteString("      });\n")
		sb.WriteString("    } catch (e) {\n")
		sb.WriteString("      return writeError(c, 400, 'INVALID_MULTIPART', 'failed to parse multipart body');\n")
		sb.WriteString("    }\n")
		sb.WriteString("  } else {\n")
		sb.WriteString("    try {\n")
		sb.WriteString("      body = await c.req.json();\n")
		sb.WriteString("    } catch (e) {\n")
		sb.WriteString("      return writeError(c, 400, 'INVALID_JSON', 'failed to parse json body');\n")
		sb.WriteString("    }\n")
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

			if !f.Nullable && f.Default == nil && f.Type != resource.TypeBlob {
				sb.WriteString(fmt.Sprintf("  if (body['%s'] === undefined || body['%s'] === null) {\n", f.Name, f.Name))
				sb.WriteString(fmt.Sprintf("    return writeError(c, 400, 'VALIDATION_FAILED', 'field %s is required');\n", f.Name))
				sb.WriteString("  }\n")
			}

			// Type Dispatch #2: IR PrimitiveType -> TS type validation
			switch f.Type {
			case resource.TypeString, resource.TypeText, resource.TypeMarkdown, resource.TypeEmail, resource.TypeURL, resource.TypePassword:
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

			// Password hashing on create
			if f.Type == resource.TypePassword {
				sb.WriteString(fmt.Sprintf("  if (body['%s'] !== undefined && body['%s'] !== null && body['%s'] !== '') {\n", f.Name, f.Name, f.Name))
				sb.WriteString(fmt.Sprintf("    body['%s'] = await hashPassword(String(body['%s']));\n", f.Name, f.Name))
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

			defaultVal := "null"
			if f.Default != nil {
				if f.Type == resource.TypeBool || f.Type == resource.TypeInt || f.Type == resource.TypeFloat {
					defaultVal = fmt.Sprintf("%v", f.Default)
				} else {
					defaultVal = fmt.Sprintf("'%v'", f.Default)
				}
			}

			// Type Dispatch #3: Value conversion for DB binding
			switch f.Type {
			case resource.TypeBool:
				vals = append(vals, fmt.Sprintf("body['%s'] !== undefined ? (body['%s'] ? 1 : 0) : %s", f.Name, f.Name, defaultVal))
			case resource.TypeBlob:
				vals = append(vals, "null")
			default:
				vals = append(vals, fmt.Sprintf("body['%s'] !== undefined ? body['%s'] : %s", f.Name, f.Name, defaultVal))
			}
		}

		if p.Timestamps {
			cols = append(cols, "\"created_at\"", "\"updated_at\"")
			bindVars = append(bindVars, "?", "?")
			vals = append(vals, "now", "now")
		}

		sb.WriteString("\n  const now = new Date().toISOString();\n")
		sb.WriteString(fmt.Sprintf("  const insertSql = `INSERT INTO \"%s\" (%s) VALUES (%s) RETURNING *`;\n", table, strings.Join(cols, ", "), strings.Join(bindVars, ", ")))
		sb.WriteString("  let created: any = null;\n")
		sb.WriteString("  try {\n")
		sb.WriteString(fmt.Sprintf("    created = await c.env.DB.prepare(insertSql).bind(%s).first<any>();\n", strings.Join(vals, ", ")))
		sb.WriteString("  } catch (err: any) {\n")
		sb.WriteString("    const errMsg = String(err?.message || err);\n")
		sb.WriteString("    if (errMsg.includes('UNIQUE constraint failed') || errMsg.includes('SQLITE_CONSTRAINT')) {\n")
		sb.WriteString("      return writeError(c, 400, 'INVALID_INPUT', `unique constraint failed: ${errMsg}`);\n")
		sb.WriteString("    }\n")
		sb.WriteString("    return writeError(c, 400, 'INVALID_INPUT', errMsg);\n")
		sb.WriteString("  }\n")

		// 1-Step Blob upload and atomic rollback
		hasBlob := false
		for _, f := range p.Fields {
			if f.Type == resource.TypeBlob && !f.Deprecated {
				hasBlob = true
				break
			}
		}

		if hasBlob {
			sb.WriteString("  if (created && formData) {\n")
			sb.WriteString("    const uploadedBlobKeys: string[] = [];\n")
			sb.WriteString("    let blobUploadError: any = null;\n")
			for _, f := range p.Fields {
				if f.Type == resource.TypeBlob && !f.Deprecated {
					blobField := f.Name
					sb.WriteString("    if (!blobUploadError) {\n")
					sb.WriteString(fmt.Sprintf("      const file_%s = formData.get('%s');\n", blobField, blobField))
					sb.WriteString(fmt.Sprintf("      if (file_%s !== null && file_%s !== undefined && file_%s !== '') {\n", blobField, blobField, blobField))
					sb.WriteString(fmt.Sprintf("        let fileData_%s: any = file_%s;\n", blobField, blobField))
					sb.WriteString(fmt.Sprintf("        let mimeType_%s = 'application/octet-stream';\n", blobField))
					sb.WriteString(fmt.Sprintf("        let ext_%s = '';\n", blobField))
					sb.WriteString(fmt.Sprintf("        if (typeof file_%s === 'object') {\n", blobField))
					sb.WriteString(fmt.Sprintf("          mimeType_%s = (file_%s as any).type || mimeType_%s;\n", blobField, blobField, blobField))
					sb.WriteString(fmt.Sprintf("          if ((file_%s as any).name) { ext_%s = (file_%s as any).name.substring((file_%s as any).name.lastIndexOf('.')); }\n", blobField, blobField, blobField, blobField))
					sb.WriteString(fmt.Sprintf("          if (typeof (file_%s as any).stream === 'function') { fileData_%s = (file_%s as any).stream(); }\n", blobField, blobField, blobField))
					sb.WriteString("        }\n")
					sb.WriteString(fmt.Sprintf("        const key = `blobs/%s/${created.id}/%s_${Date.now()}${ext_%s}`;\n", table, blobField, blobField))
					sb.WriteString("        try {\n")
					sb.WriteString(fmt.Sprintf("          await c.env.BUCKET.put(key, fileData_%s, { httpMetadata: { contentType: mimeType_%s } });\n", blobField, blobField))
					sb.WriteString("          uploadedBlobKeys.push(key);\n")
					sb.WriteString(fmt.Sprintf("          await c.env.DB.prepare('UPDATE \"%s\" SET \"%s\" = ? WHERE id = ?').bind(key, created.id).run();\n", table, blobField))
					sb.WriteString(fmt.Sprintf("          created['%s'] = key;\n", blobField))
					sb.WriteString("        } catch (err) {\n")
					sb.WriteString("          blobUploadError = err;\n")
					sb.WriteString("        }\n")
					sb.WriteString("      }\n")
					sb.WriteString("    }\n")
				}
			}
			sb.WriteString("    if (blobUploadError) {\n")
			sb.WriteString("      // Order rationale: Execute D1 hard delete BEFORE R2 compensating deletion to ensure\n")
			sb.WriteString("      // HTTP GET requests immediately return 404 NOT_FOUND instead of 200 OK with a broken image link\n")
			sb.WriteString("      // (dangling reference) while R2 orphan objects are being deleted.\n")
			sb.WriteString("      let d1RollbackFailed = false;\n")
			sb.WriteString("      try {\n")
			sb.WriteString(fmt.Sprintf("        await c.env.DB.prepare('DELETE FROM \"%s\" WHERE id = ?').bind(created.id).run();\n", table))
			sb.WriteString("      } catch (rollbackErr) {\n")
			sb.WriteString("        d1RollbackFailed = true;\n")
			sb.WriteString("      }\n")
			sb.WriteString("      const failedCleanupKeys: string[] = [];\n")
			sb.WriteString("      for (const key of uploadedBlobKeys) {\n")
			sb.WriteString("        try {\n")
			sb.WriteString("          await c.env.BUCKET.delete(key);\n")
			sb.WriteString("        } catch (cleanupErr) {\n")
			sb.WriteString("          failedCleanupKeys.push(key);\n")
			sb.WriteString("        }\n")
			sb.WriteString("      }\n")
			sb.WriteString("      if (failedCleanupKeys.length > 0) {\n")
			sb.WriteString("        return writeError(c, 500, 'BLOB_ORPHAN_CLEANUP_FAILED', 'failed uploading blob; some R2 orphan objects could not be cleaned up', { orphan_keys: failedCleanupKeys, d1_rollback_failed: d1RollbackFailed });\n")
			sb.WriteString("      } else if (d1RollbackFailed) {\n")
			sb.WriteString("        return writeError(c, 500, 'BLOB_STORE_FAILED_RECORD_PRESERVED', 'failed uploading blob and failed rolling back record');\n")
			sb.WriteString("      } else {\n")
			sb.WriteString("        return writeError(c, 500, 'BLOB_STORE_FAILED', 'failed uploading blob; record creation rolled back');\n")
			sb.WriteString("      }\n")
			sb.WriteString("    }\n")
			sb.WriteString("  }\n")
		}

		sb.WriteString(fmt.Sprintf("  return c.json({ data: sanitizeRecord(created, %s) }, 201);\n", pwdFieldsJS))
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
			sb.WriteString(fmt.Sprintf("  const ownerVal = (existing as any)['%s'];\n", ownershipField))
			sb.WriteString("  if (ownerVal === null || ownerVal === undefined) {\n")
			sb.WriteString("    if (authUser.role !== 'admin') {\n")
			sb.WriteString("      return writeError(c, 403, 'FORBIDDEN', 'forbidden');\n")
			sb.WriteString("    }\n")
			sb.WriteString("  } else if (authUser.role !== 'admin' && ownerVal != authUser.id) {\n")
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

		// Password hashing on update
		for _, f := range p.Fields {
			if f.Type == resource.TypePassword {
				sb.WriteString(fmt.Sprintf("  if (body['%s'] !== undefined && body['%s'] !== null && body['%s'] !== '') {\n", f.Name, f.Name, f.Name))
				sb.WriteString(fmt.Sprintf("    body['%s'] = await hashPassword(String(body['%s']));\n", f.Name, f.Name))
				sb.WriteString("  } else {\n")
				sb.WriteString(fmt.Sprintf("    body['%s'] = (existing as any)['%s'];\n", f.Name, f.Name))
				sb.WriteString("  }\n")
			}
		}

		setClauses := []string{}
		updateVals := []string{}
		for _, f := range p.Fields {
			if f.Deprecated {
				continue
			}
			setClauses = append(setClauses, fmt.Sprintf("\"%s\" = ?", f.Name))
			updateVals = append(updateVals, fmt.Sprintf("body['%s'] !== undefined ? body['%s'] : (existing as any)['%s']", f.Name, f.Name, f.Name))
		}
		if p.Timestamps {
			setClauses = append(setClauses, "\"updated_at\" = ?")
			updateVals = append(updateVals, "now")
		}
		updateVals = append(updateVals, "id")

		sb.WriteString("  const now = new Date().toISOString();\n")
		sb.WriteString(fmt.Sprintf("  const updateSql = `UPDATE \"%s\" SET %s WHERE id = ?%s RETURNING *`;\n", table, strings.Join(setClauses, ", "), softCond))
		sb.WriteString("  let updated: any = null;\n")
		sb.WriteString("  try {\n")
		sb.WriteString(fmt.Sprintf("    updated = await c.env.DB.prepare(updateSql).bind(%s).first();\n", strings.Join(updateVals, ", ")))
		sb.WriteString("  } catch (err: any) {\n")
		sb.WriteString("    const errMsg = String(err?.message || err);\n")
		sb.WriteString("    if (errMsg.includes('UNIQUE constraint failed') || errMsg.includes('SQLITE_CONSTRAINT')) {\n")
		sb.WriteString("      return writeError(c, 400, 'INVALID_INPUT', `unique constraint failed: ${errMsg}`);\n")
		sb.WriteString("    }\n")
		sb.WriteString("    return writeError(c, 400, 'INVALID_INPUT', errMsg);\n")
		sb.WriteString("  }\n")
		sb.WriteString("  if (!updated) {\n")
		sb.WriteString("    return writeError(c, 404, 'NOT_FOUND', 'record not found');\n")
		sb.WriteString("  }\n")
		sb.WriteString(fmt.Sprintf("  return c.json({ data: sanitizeRecord(updated, %s) });\n", pwdFieldsJS))
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
			sb.WriteString(fmt.Sprintf("  const ownerVal = (existing as any)['%s'];\n", ownershipField))
			sb.WriteString("  if (ownerVal === null || ownerVal === undefined) {\n")
			sb.WriteString("    if (authUser.role !== 'admin') {\n")
			sb.WriteString("      return writeError(c, 403, 'FORBIDDEN', 'forbidden');\n")
			sb.WriteString("    }\n")
			sb.WriteString("  } else if (authUser.role !== 'admin' && ownerVal != authUser.id) {\n")
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

		// 6. GET /view/{table} (HTML List View)
		sb.WriteString(fmt.Sprintf("\n// VIEW LIST /view/%s\n", table))
		sb.WriteString(fmt.Sprintf("app.get('/view/%s', async (c) => {\n", table))
		sb.WriteString("  const authUser = await getAuthUser(c);\n")
		sb.WriteString(fmt.Sprintf("  const { results } = await c.env.DB.prepare('SELECT * FROM \"%s\"%s ORDER BY id ASC').all();\n", table, whereClause))
		sb.WriteString(fmt.Sprintf("  let html = `<!DOCTYPE html><html><head><title>%s List</title></head><body>`;\n", p.ResourceName))
		sb.WriteString(fmt.Sprintf("  html += `<h1>%s List</h1>`;\n", p.ResourceName))
		sb.WriteString(fmt.Sprintf("  html += `<a href=\"/view/%s/new\">+ New %s</a><br/><br/><table border=\"1\"><thead><tr><th>id</th>`;\n", table, p.ResourceName))
		for _, f := range p.Fields {
			if f.Deprecated || f.Type == resource.TypePassword {
				continue
			}
			sb.WriteString(fmt.Sprintf("  html += `<th>%s</th>`;\n", f.Name))
		}
		sb.WriteString("  html += `<th>Actions</th></tr></thead><tbody>`;\n")
		sb.WriteString("  for (const row of (results || [])) {\n")
		sb.WriteString("    html += `<tr><td>${(row as any).id}</td>`;\n")
		for _, f := range p.Fields {
			if f.Deprecated || f.Type == resource.TypePassword {
				continue
			}
			sb.WriteString(fmt.Sprintf("    html += `<td>${escapeHTML((row as any)['%s'])}</td>`;\n", f.Name))
		}
		sb.WriteString(fmt.Sprintf("    html += `<td><a href=\"/view/%s/${(row as any).id}\">Detail</a> <a href=\"/view/%s/${(row as any).id}/edit\">Edit</a></td></tr>`;\n", table, table))
		sb.WriteString("  }\n")
		sb.WriteString("  html += `</tbody></table></body></html>`;\n")
		sb.WriteString("  return c.html(html);\n")
		sb.WriteString("});\n")

		// 7. GET /view/{table}/new (HTML Form New)
		sb.WriteString(fmt.Sprintf("\n// VIEW NEW /view/%s/new\n", table))
		sb.WriteString(fmt.Sprintf("app.get('/view/%s/new', async (c) => {\n", table))
		sb.WriteString(fmt.Sprintf("  let html = `<!DOCTYPE html><html><head><title>New %s</title></head><body><h1>New %s</h1><form method=\"POST\" action=\"/view/%s\">`;\n", p.ResourceName, p.ResourceName, table))
		for _, f := range p.Fields {
			if f.Deprecated || f.Type == resource.TypePassword {
				continue
			}
			inputType := "text"
			if f.Type == resource.TypeInt || f.Type == resource.TypeFloat || f.IsDerivedFK {
				inputType = "number"
			}
			if f.Type == resource.TypeText || f.Type == resource.TypeMarkdown {
				sb.WriteString(fmt.Sprintf("  html += `<label>%s: <textarea name=\"%s\"></textarea></label><br/><br/>`;\n", f.Name, f.Name))
			} else {
				sb.WriteString(fmt.Sprintf("  html += `<label>%s: <input type=\"%s\" name=\"%s\" /></label><br/><br/>`;\n", f.Name, inputType, f.Name))
			}
		}
		sb.WriteString("  html += `<button type=\"submit\">Save</button></form></body></html>`;\n")
		sb.WriteString("  return c.html(html);\n")
		sb.WriteString("});\n")

		// 8. POST /view/{table} (HTML Create Submit)
		sb.WriteString(fmt.Sprintf("\n// VIEW CREATE SUBMIT /view/%s\n", table))
		sb.WriteString(fmt.Sprintf("app.post('/view/%s', async (c) => {\n", table))
		sb.WriteString("  const formData = await c.req.formData();\n")
		sb.WriteString("  const body: any = {};\n")
		sb.WriteString("  formData.forEach((value, key) => { body[key] = value; });\n")
		sb.WriteString("  const now = new Date().toISOString();\n")
		sb.WriteString(fmt.Sprintf("  await c.env.DB.prepare(insertSql).bind(%s).run();\n", strings.Join(vals, ", ")))
		sb.WriteString(fmt.Sprintf("  return c.redirect('/view/%s', 303);\n", table))
		sb.WriteString("});\n")

		// 9. GET /view/{table}/:id (HTML Detail View with XSS Sanitization)
		sb.WriteString(fmt.Sprintf("\n// VIEW DETAIL /view/%s/:id\n", table))
		sb.WriteString(fmt.Sprintf("app.get('/view/%s/:id', async (c) => {\n", table))
		sb.WriteString("  const id = c.req.param('id');\n")
		sb.WriteString(fmt.Sprintf("  const record = await c.env.DB.prepare('SELECT * FROM \"%s\" WHERE id = ?%s').bind(id).first<any>();\n", table, softCond))
		sb.WriteString("  if (!record) return c.html('<h1>404 Not Found</h1>', 404);\n")
		sb.WriteString(fmt.Sprintf("  let html = `<!DOCTYPE html><html><head><title>%s Detail</title></head><body><h1>%s #${id}</h1><dl>`;\n", p.ResourceName, p.ResourceName))
		for _, f := range p.Fields {
			if f.Deprecated || f.Type == resource.TypePassword {
				continue
			}
			if f.Type == resource.TypeMarkdown {
				sb.WriteString(fmt.Sprintf("  html += `<dt>%s</dt><dd>${sanitizeHTML(String(record['%s'] || ''))}</dd>`;\n", f.Name, f.Name))
			} else {
				sb.WriteString(fmt.Sprintf("  html += `<dt>%s</dt><dd>${escapeHTML(record['%s'])}</dd>`;\n", f.Name, f.Name))
			}
		}
		sb.WriteString("  html += `</dl></body></html>`;\n")
		sb.WriteString("  return c.html(html);\n")
		sb.WriteString("});\n")

		// 10. Blob endpoints for any blob fields
		for _, f := range p.Fields {
			if f.Type != resource.TypeBlob || f.Deprecated {
				continue
			}
			blobField := f.Name

			// 10a. POST /api/{table}/:id/upload/{field} (2-Step Overwrite Upload)
			sb.WriteString(fmt.Sprintf("\n// OVERWRITE UPLOAD /api/%s/:id/upload/%s\n", table, blobField))
			sb.WriteString(fmt.Sprintf("app.post('%s/:id/upload/%s', async (c) => {\n", endpoint, blobField))
			sb.WriteString("  const authUser = await getAuthUser(c);\n")
			if permUpdate != "public" {
				sb.WriteString("  if (!authUser) {\n")
				sb.WriteString("    return writeError(c, 401, 'UNAUTHORIZED', 'authentication required');\n")
				sb.WriteString("  }\n")
			}
			sb.WriteString("  const id = c.req.param('id');\n")
			sb.WriteString(fmt.Sprintf("  const existing = await c.env.DB.prepare('SELECT * FROM \"%s\" WHERE id = ?%s').bind(id).first<any>();\n", table, softCond))
			sb.WriteString("  if (!existing) {\n")
			sb.WriteString("    return writeError(c, 404, 'NOT_FOUND', 'record not found');\n")
			sb.WriteString("  }\n")
			if permUpdate == "owner" && ownershipField != "" {
				sb.WriteString(fmt.Sprintf("  const ownerVal = existing['%s'];\n", ownershipField))
				sb.WriteString("  if (ownerVal === null || ownerVal === undefined) {\n")
				sb.WriteString("    if (authUser.role !== 'admin') {\n")
				sb.WriteString("      return writeError(c, 403, 'FORBIDDEN', 'forbidden');\n")
				sb.WriteString("    }\n")
				sb.WriteString("  } else if (authUser.role !== 'admin' && ownerVal != authUser.id) {\n")
				sb.WriteString("    return writeError(c, 403, 'FORBIDDEN', 'forbidden');\n")
				sb.WriteString("  }\n")
			} else if strings.HasPrefix(permUpdate, "role:") {
				role := strings.TrimPrefix(permUpdate, "role:")
				sb.WriteString(fmt.Sprintf("  if (!authUser || authUser.role !== '%s') {\n", role))
				sb.WriteString("    return writeError(c, 403, 'FORBIDDEN', 'forbidden');\n")
				sb.WriteString("  }\n")
			}

			sb.WriteString("  const formData = await c.req.formData().catch(() => null);\n")
			sb.WriteString(fmt.Sprintf("  const file = formData ? formData.get('%s') as File : null;\n", blobField))
			sb.WriteString("  if (!file) {\n")
			sb.WriteString("    return writeError(c, 400, 'VALIDATION_FAILED', 'missing file payload');\n")
			sb.WriteString("  }\n")
			sb.WriteString("  const ext = file.name ? file.name.substring(file.name.lastIndexOf('.')) : '';\n")
			sb.WriteString(fmt.Sprintf("  const key = `blobs/%s/${id}/%s_${Date.now()}${ext}`;\n", table, blobField))
			sb.WriteString("  await c.env.BUCKET.put(key, file.stream(), { httpMetadata: { contentType: file.type } });\n")
			sb.WriteString(fmt.Sprintf("  await c.env.DB.prepare('UPDATE \"%s\" SET \"%s\" = ? WHERE id = ?').bind(key, id).run();\n", table, blobField))
			sb.WriteString(fmt.Sprintf("  return c.json({ data: { %s: key } });\n", blobField))
			sb.WriteString("});\n")

			// 10b. GET /api/{table}/:id/blob/{field} (Download)
			sb.WriteString(fmt.Sprintf("\n// DOWNLOAD BLOB /api/%s/:id/blob/%s\n", table, blobField))
			sb.WriteString(fmt.Sprintf("app.get('%s/:id/blob/%s', async (c) => {\n", endpoint, blobField))
			sb.WriteString("  const authUser = await getAuthUser(c);\n")
			if permRead != "public" && !(permRead == "owner" && ownershipField != "") {
				sb.WriteString("  if (!authUser) {\n")
				sb.WriteString("    return writeError(c, 401, 'UNAUTHORIZED', 'authentication required');\n")
				sb.WriteString("  }\n")
			}
			sb.WriteString("  const id = c.req.param('id');\n")
			sb.WriteString(fmt.Sprintf("  const record = await c.env.DB.prepare('SELECT * FROM \"%s\" WHERE id = ?%s').bind(id).first<any>();\n", table, softCond))
			sb.WriteString("  if (!record) {\n")
			sb.WriteString("    return writeError(c, 404, 'NOT_FOUND', 'record not found');\n")
			sb.WriteString("  }\n")
			if permRead == "owner" && ownershipField != "" {
				sb.WriteString(fmt.Sprintf("  const ownerVal = record['%s'];\n", ownershipField))
				sb.WriteString("  if (ownerVal !== null && ownerVal !== undefined) {\n")
				sb.WriteString("    if (!authUser) {\n")
				sb.WriteString("      return writeError(c, 401, 'UNAUTHORIZED', 'authentication required');\n")
				sb.WriteString("    }\n")
				sb.WriteString("    if (authUser.role !== 'admin' && ownerVal != authUser.id) {\n")
				sb.WriteString("      return writeError(c, 403, 'FORBIDDEN', 'forbidden');\n")
				sb.WriteString("    }\n")
				sb.WriteString("  }\n")
			} else if strings.HasPrefix(permRead, "role:") {
				role := strings.TrimPrefix(permRead, "role:")
				sb.WriteString(fmt.Sprintf("  if (!authUser || authUser.role !== '%s') {\n", role))
				sb.WriteString("    return writeError(c, 403, 'FORBIDDEN', 'forbidden');\n")
				sb.WriteString("  }\n")
			}
			sb.WriteString(fmt.Sprintf("  const key = record['%s'];\n", blobField))
			sb.WriteString("  if (!key) {\n")
			sb.WriteString("    return writeError(c, 404, 'NOT_FOUND', 'blob key not found');\n")
			sb.WriteString("  }\n")
			sb.WriteString("  const object = await c.env.BUCKET.get(key);\n")
			sb.WriteString("  if (!object) {\n")
			sb.WriteString("    return writeError(c, 404, 'NOT_FOUND', 'blob object not found in R2');\n")
			sb.WriteString("  }\n")
			sb.WriteString("  c.header('Content-Type', object.httpMetadata?.contentType || 'application/octet-stream');\n")
			sb.WriteString("  return c.body(object.body);\n")
			sb.WriteString("});\n")

			// 10c. DELETE /api/{table}/:id/blob/{field} (Delete Blob)
			sb.WriteString(fmt.Sprintf("\n// DELETE BLOB /api/%s/:id/blob/%s\n", table, blobField))
			sb.WriteString(fmt.Sprintf("app.delete('%s/:id/blob/%s', async (c) => {\n", endpoint, blobField))
			sb.WriteString("  const authUser = await getAuthUser(c);\n")
			if permDelete != "public" {
				sb.WriteString("  if (!authUser) {\n")
				sb.WriteString("    return writeError(c, 401, 'UNAUTHORIZED', 'authentication required');\n")
				sb.WriteString("  }\n")
			}
			sb.WriteString("  const id = c.req.param('id');\n")
			sb.WriteString(fmt.Sprintf("  const record = await c.env.DB.prepare('SELECT * FROM \"%s\" WHERE id = ?%s').bind(id).first<any>();\n", table, softCond))
			sb.WriteString("  if (!record) {\n")
			sb.WriteString("    return writeError(c, 404, 'NOT_FOUND', 'record not found');\n")
			sb.WriteString("  }\n")
			if permDelete == "owner" && ownershipField != "" {
				sb.WriteString(fmt.Sprintf("  const ownerVal = record['%s'];\n", ownershipField))
				sb.WriteString("  if (ownerVal === null || ownerVal === undefined) {\n")
				sb.WriteString("    if (authUser.role !== 'admin') {\n")
				sb.WriteString("      return writeError(c, 403, 'FORBIDDEN', 'forbidden');\n")
				sb.WriteString("    }\n")
				sb.WriteString("  } else if (authUser.role !== 'admin' && ownerVal != authUser.id) {\n")
				sb.WriteString("    return writeError(c, 403, 'FORBIDDEN', 'forbidden');\n")
				sb.WriteString("  }\n")
			} else if strings.HasPrefix(permDelete, "role:") {
				role := strings.TrimPrefix(permDelete, "role:")
				sb.WriteString(fmt.Sprintf("  if (!authUser || authUser.role !== '%s') {\n", role))
				sb.WriteString("    return writeError(c, 403, 'FORBIDDEN', 'forbidden');\n")
				sb.WriteString("  }\n")
			}
			sb.WriteString(fmt.Sprintf("  const key = record['%s'];\n", blobField))
			sb.WriteString("  if (key) {\n")
			sb.WriteString("    await c.env.BUCKET.delete(key);\n")
			sb.WriteString(fmt.Sprintf("    await c.env.DB.prepare('UPDATE \"%s\" SET \"%s\" = NULL WHERE id = ?').bind(id).run();\n", table, blobField))
			sb.WriteString("  }\n")
			sb.WriteString("  return c.json({ data: { deleted: true } });\n")
			sb.WriteString("});\n")
		}
	}

	sb.WriteString(`
// LOGIN
app.post('/login', async (c) => {
  let username = '';
  let password = '';
  const contentType = c.req.header('Content-Type') || '';
  if (contentType.includes('application/json')) {
    const body = await c.req.json().catch(() => ({}));
    username = body.username || body.email || '';
    password = body.password || '';
  } else {
    const formData = await c.req.formData().catch(() => new FormData());
    username = (formData.get('username') || formData.get('email') || '').toString();
    password = (formData.get('password') || '').toString();
  }

  if (!username || !password) {
    return writeError(c, 400, 'VALIDATION_FAILED', 'username and password are required');
  }

  let user: any = null;
  try {
    user = await c.env.DB.prepare('SELECT * FROM "users" WHERE email = ? AND ("deleted_at" IS NULL OR "deleted_at" = \'\')').bind(username).first<any>();
  } catch (e) {}

  if (!user || !(await verifyPassword(password, user.password))) {
    return writeError(c, 401, 'INVALID_CREDENTIALS', 'invalid email or password');
  }

  const sessionId = crypto.randomUUID();
  const now = new Date();
  const expiresAt = new Date(now.getTime() + 24 * 60 * 60 * 1000).toISOString();
  try {
    await c.env.DB.prepare('INSERT INTO "_mold_sessions" (id, user_id, created_at, expires_at) VALUES (?, ?, ?, ?)').bind(sessionId, user.id, now.toISOString(), expiresAt).run();
  } catch (e) {}

  c.header('Set-Cookie', 'mold_session=' + sessionId + '; Path=/; HttpOnly; SameSite=Lax');
  return c.json({ data: { user: sanitizeRecord(user, ['password']), session_id: sessionId } });
});

// LOGOUT
app.post('/logout', async (c) => {
  const cookieHeader = c.req.header('Cookie') || '';
  const match = cookieHeader.match(/mold_session=([^;]+)/);
  if (match) {
    const token = match[1];
    try {
      await c.env.DB.prepare('DELETE FROM "_mold_sessions" WHERE id = ?').bind(token).run();
    } catch (e) {}
  }
  c.header('Set-Cookie', 'mold_session=; Path=/; HttpOnly; Max-Age=0');
  return c.json({ data: { logged_out: true } });
});

export default app;
`)
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
  ],
  "r2_buckets": [
    {
      "binding": "BUCKET",
      "bucket_name": "mold-r2"
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

