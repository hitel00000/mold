package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/hitel00000/mold/codegen/cloudflare"
	"github.com/hitel00000/mold/resource"
	"github.com/hitel00000/mold/runtime"
)

const version = "0.1.0"

func printUsage() {
	fmt.Printf(`Mold CLI v%s — Opinionated Resource Runtime for Online Services

Usage:
  mold <command> [flags]

Commands:
  codegen, gen    Generate typed target code (e.g. Cloudflare Workers TS) from Resource YAML
  serve, server   Run production/standalone Mold HTTP server
  version, -v     Print Mold version

Flags for 'mold codegen':
  -target, -t     Target code generation platform (default: "cloudflare")
  -dir, -d        Path to Resource YAML directory (default: "./resources")
  -out, -o        Output path for generated TypeScript/target file (default: "./generated/mold_app.ts")
  -schema-out     Optional output path for generated Schema SQL (e.g. "./schema.sql")
  -package-out    Optional output path for generated package.json
  -wrangler-out   Optional output path for generated wrangler.jsonc

Flags for 'mold serve':
  -dir, -d        Path to Resource YAML directory (default: "./resources")
  -db             Path to SQLite database file (default: "./mold.db")
  -port, -p       Port to listen on (default: 8080)

Examples:
  mold codegen -d ./resources -o ./functions/_shared/generated/mold_app.ts
  mold codegen --target cloudflare --dir ./resources --out ./dist/app.ts --schema-out ./schema.sql
  mold serve -d ./resources -db ./app.db -p 3000
`, version)
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(0)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	case "codegen", "gen":
		runCodegen(args)
	case "serve", "server":
		runServe(args)
	case "version", "-v", "--version":
		fmt.Printf("Mold v%s\n", version)
	case "help", "-h", "--help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\nRun 'mold help' for usage.\n", cmd)
		os.Exit(1)
	}
}

func runCodegen(args []string) {
	fs := flag.NewFlagSet("codegen", flag.ExitOnError)

	var target, dir, out, schemaOut, packageOut, wranglerOut string

	fs.StringVar(&target, "target", "cloudflare", "Target code generation platform")
	fs.StringVar(&target, "t", "cloudflare", "Target code generation platform (shorthand)")

	fs.StringVar(&dir, "dir", "./resources", "Path to Resource YAML directory")
	fs.StringVar(&dir, "d", "./resources", "Path to Resource YAML directory (shorthand)")

	fs.StringVar(&out, "out", "./generated/mold_app.ts", "Output path for generated TypeScript file")
	fs.StringVar(&out, "o", "./generated/mold_app.ts", "Output path for generated TypeScript file (shorthand)")

	fs.StringVar(&schemaOut, "schema-out", "", "Optional output path for generated Schema SQL")
	fs.StringVar(&packageOut, "package-out", "", "Optional output path for generated package.json")
	fs.StringVar(&wranglerOut, "wrangler-out", "", "Optional output path for generated wrangler.jsonc")

	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing flags: %v\n", err)
		os.Exit(1)
	}

	// 1. Load and validate Resource YAML files
	absDir, err := filepath.Abs(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid resource directory path '%s': %v\n", dir, err)
		os.Exit(1)
	}

	reg, err := resource.LoadAll(absDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading resources from '%s': %v\n", absDir, err)
		os.Exit(1)
	}

	resources := reg.List()
	if len(resources) == 0 {
		fmt.Fprintf(os.Stderr, "No YAML resources found in '%s'\n", absDir)
		os.Exit(1)
	}

	// 2. Generate target code
	switch target {
	case "cloudflare", "cf", "hono":
		gen := cloudflare.NewGenerator()
		output, err := gen.Generate(reg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error generating Cloudflare Workers code: %v\n", err)
			os.Exit(1)
		}

		// Write primary output (TS code)
		if err := writeFile(out, output.IndexTS); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to write output to '%s': %v\n", out, err)
			os.Exit(1)
		}
		log.Printf("[mold codegen] Generated TypeScript code -> %s", out)

		// Optional schema SQL output
		if schemaOut != "" {
			if err := writeFile(schemaOut, output.SchemaSQL); err != nil {
				fmt.Fprintf(os.Stderr, "Failed to write schema SQL to '%s': %v\n", schemaOut, err)
				os.Exit(1)
			}
			log.Printf("[mold codegen] Generated D1 Schema SQL -> %s", schemaOut)
		}

		// Optional package.json output
		if packageOut != "" {
			if err := writeFile(packageOut, output.PackageJSON); err != nil {
				fmt.Fprintf(os.Stderr, "Failed to write package.json to '%s': %v\n", packageOut, err)
				os.Exit(1)
			}
			log.Printf("[mold codegen] Generated package.json -> %s", packageOut)
		}

		// Optional wrangler config output
		if wranglerOut != "" {
			if err := writeFile(wranglerOut, output.WranglerConfig); err != nil {
				fmt.Fprintf(os.Stderr, "Failed to write wrangler config to '%s': %v\n", wranglerOut, err)
				os.Exit(1)
			}
			log.Printf("[mold codegen] Generated wrangler.jsonc -> %s", wranglerOut)
		}

		log.Printf("[mold codegen] Successfully processed %d resources.", len(resources))

	default:
		fmt.Fprintf(os.Stderr, "Unsupported codegen target '%s'. Supported targets: cloudflare\n", target)
		os.Exit(1)
	}
}

func runServe(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)

	var dir, dbPath string
	var port int

	fs.StringVar(&dir, "dir", "./resources", "Path to Resource YAML directory")
	fs.StringVar(&dir, "d", "./resources", "Path to Resource YAML directory (shorthand)")
	fs.StringVar(&dbPath, "db", "./mold.db", "Path to SQLite database file")
	fs.IntVar(&port, "port", 8080, "Port to listen on")
	fs.IntVar(&port, "p", 8080, "Port to listen on (shorthand)")

	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing flags: %v\n", err)
		os.Exit(1)
	}

	cfg := runtime.Config{ResourceDir: dir, DBPath: dbPath}
	app, err := runtime.New(cfg)
	if err != nil {
		log.Fatalf("Failed to bootstrap Mold runtime: %v", err)
	}
	defer app.Close()

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	log.Printf("[mold serve] Starting Mold server on http://%s ...", addr)
	if err := http.ListenAndServe(addr, app); err != nil {
		log.Fatalf("[mold serve] Server error: %v", err)
	}
}

func writeFile(path, content string) error {
	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, []byte(content), 0644)
}
