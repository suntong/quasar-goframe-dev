package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

/*
================================================================================
SCHEMA ARCHITECT: GoFrame Relation Parser (AST Edition)
================================================================================
This tool performs holistic static analysis of GoFrame 'do' models.
It maps the "Database-First" relations defined via 'with' tags.

TARGET STRUCTURE:
The parser specifically targets GoFrame's standard scaffold:
`internal/model/do/*.go` or `internal/app/* /model/do/*.go`

USAGE FOR CODE GENERATION:
Run as a standalone tool or integrate into build scripts (e.g., go generate).
Output SchemaMap can be JSON-marshaled and fed into text/template for UI gen:
- 1:1: Detail views/forms
- 1:N: Sub-grids/tables
================================================================================
*/

// SchemaMap is a collection of metadata for all discovered tables.
// Key: Struct Name (e.g., "User"), Value: Metadata object.
type SchemaMap map[string]*TableMetadata

// TableMetadata contains the structural summary of a DB table derived from a 'do' struct.
type TableMetadata struct {
	StructName     string          // The original Go struct name (e.g., "UserCreateReq")
	NormalizedName string          // Logical entity name after removing common API suffixes/prefixes (e.g., "User")
	Source         string          // Provenance marker (e.g., "go:do", "go:api", "openapi", "merged")
	Columns        []ColumnInfo    // Captured fields for full ERD visualization and form generation
	Relations      []*RelationNode // All discovered 'with' associations
	Operations     []OperationInfo // OpenAPI operations that can be associated with this logical entity
}

// FieldConstraints captures machine-usable validation/shape constraints.
// The struct is intentionally typed so UI generation can derive rules deterministically.
type FieldConstraints struct {
	Required  bool
	Nullable  bool
	MinLength *int
	MaxLength *int
	Minimum   *float64
	Maximum   *float64
	Pattern   string
	Format    string
	Enum      []string
}

// ColumnInfo represents a non-relational field in the struct (DB Column).
type ColumnInfo struct {
	Name        string            // Go field name or OpenAPI property name
	JSONName    string            // json tag name (when available) or OpenAPI property name
	Type        string            // Go-ish type name for diagramming and generator decisions
	Validation  string            // Gvalid rules (e.g., "required|length:6,30")
	Description string            // Field description/label (e.g., "User login name")
	Additional  string            // Extra metadata (e.g., placeholders or custom hints)
	Constraints *FieldConstraints // OpenAPI-derived constraints
	Ref         string            // OpenAPI $ref target schema name (if the field is a component reference)
	IsArray     bool              // True if OpenAPI type is array or Go slice
	Source      string            // Provenance marker (e.g., "go:do", "go:api", "openapi")
}

// RelationNode defines a single relationship between two tables.
// Designed to be fed into text/template for UI generation (e.g., auto-rendering sub-grids).
type RelationNode struct {
	FieldName    string `json:"field_name"`    // Struct field (e.g., "UserDetail")
	TargetStruct string `json:"target_struct"` // Related struct (e.g., "UserDetail")
	IsCollection bool   `json:"is_collection"` // True if Slice (1:N), False if Struct (1:1)
	TargetKey    string `json:"target_key"`    // The FK on the remote table (the 'uid' in 'uid=id')
	SourceKey    string `json:"source_key"`    // The PK on the local table (the 'id' in 'uid=id')
	Validation   string `json:"validation"`    // Relation-specific validation
	Description  string `json:"description"`   // Relation-specific description
}

// OperationInfo is a minimal OpenAPI operation descriptor used by UI generators.
type OperationInfo struct {
	Method         string   `json:"method"`
	Path           string   `json:"path"`
	OperationID    string   `json:"operation_id"`
	Summary        string   `json:"summary"`
	Tags           []string `json:"tags"`
	RequestSchema  string   `json:"request_schema"`
	ResponseSchema string   `json:"response_schema"`
	Source         string   `json:"source"` // "openapi"
}

// ConsolidatedSchema is a generator-friendly container that provides both
// random-access (map) and stable iteration order (list).
type ConsolidatedSchema struct {
	Entities    map[string]*TableMetadata `json:"entities"`
	EntityList  []*TableMetadata          `json:"entity_list"`
	GeneratedBy string                    `json:"generated_by"`
}

func main() {
	// CONFIGURATION: Adjust searchRoot to match your project structure.
	// Common GoFrame paths: "./internal/model/do" or "./internal" to include api/
	var (
		searchRoot  = flag.String("root", "./internal", "Root directory to scan for GoFrame structs (internal/...)")
		openapiPath = flag.String("openapi", "", "Path to OpenAPI v3 JSON (optional)")
		rawOutPath  = flag.String("raw-out", "", "Write raw (unconsolidated) schema JSON (optional)")
		outPath     = flag.String("out", "schema.logical.json", "Write consolidated schema JSON")
	)
	flag.Parse()

	schema := make(SchemaMap)

	fmt.Printf("🔍 Scanning %s for GoFrame 'do' models and API structs...\n", *searchRoot)

	err := filepath.Walk(*searchRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err // Propagate errors
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}

		// Normalize paths for cross-platform reliability.
		// Scan both model/do and api directories (common in GoFrame projects).
		pathSlash := filepath.ToSlash(path)
		if !strings.Contains(pathSlash, "/model/do") && !strings.Contains(pathSlash, "/api") {
			return nil
		}

		parseFile(path, schema)
		return nil
	})
	if err != nil {
		fmt.Printf("❌ Error: %v\n", err)
		os.Exit(1)
	}

	if *openapiPath != "" {
		fmt.Printf("📦 Loading OpenAPI: %s\n", *openapiPath)
		openapiSchema, err := parseOpenAPIFile(*openapiPath)
		if err != nil {
			fmt.Printf("❌ OpenAPI error: %v\n", err)
			os.Exit(1)
		}
		for _, meta := range openapiSchema {
			putSchema(schema, meta)
		}
	}

	printSchemaSummary(schema)
	// Generate and print Mermaid ER Diagram for visualization.
	fmt.Println(generateERDiagram(schema))

	if *rawOutPath != "" {
		if err := writeJSONFile(*rawOutPath, schema); err != nil {
			fmt.Printf("❌ Error writing raw schema JSON: %v\n", err)
			os.Exit(1)
		}
	}

	consolidated := consolidateByNormalizedName(schema)
	if err := writeJSONFile(*outPath, consolidated); err != nil {
		fmt.Printf("❌ Error writing consolidated schema JSON: %v\n", err)
		os.Exit(1)
	}
}

// normalizeEntityName extracts the logical entity name by removing common
// GoFrame API request suffixes and minimal prefixes.
// Designed to be conservative and aligned with typical GoFrame naming conventions.
func normalizeEntityName(name string) string {
	cleaned := name

	// Common API request suffixes in GoFrame projects
	suffixes := []string{
		"Req", "Request", "Res", "Response", "Input", "Output",
		"Create", "Update", "Add", "Edit", "Delete",
		"Item", "Detail", "List", "Get", "Query", "Form",
		"Dto", "DTO",
	}

	// Very limited prefixes — most GoFrame projects avoid heavy prefixing
	prefixes := []string{
		"V1", "V2", "Api",
	}

	for _, pre := range prefixes {
		cleaned = strings.TrimPrefix(cleaned, pre)
	}
	for _, suf := range suffixes {
		cleaned = strings.TrimSuffix(cleaned, suf)
	}

	// Minimal plural handling (common in entity names)
	cleaned = strings.TrimSuffix(cleaned, "s")

	if cleaned == "" {
		return name // fallback to original if normalization removes everything
	}

	return cleaned
}

func sourceFromPath(path string) string {
	pathSlash := filepath.ToSlash(path)
	if strings.Contains(pathSlash, "/model/do") {
		return "go:do"
	}
	if strings.Contains(pathSlash, "/api") {
		return "go:api"
	}
	return "go"
}

func putSchema(schema SchemaMap, table *TableMetadata) {
	// SchemaMap keys are required to be unique to prevent accidental overwrites
	// when multiple sources provide the same struct/schema name.
	base := table.StructName
	key := base
	for i := 2; ; i++ {
		if _, exists := schema[key]; !exists {
			schema[key] = table
			return
		}
		key = fmt.Sprintf("%s__%d", base, i)
	}
}

// parseFile uses the go/ast package to read source code without executing it.
func parseFile(path string, schema SchemaMap) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		fmt.Printf("⚠️ Skipping %s: %v\n", path, err)
		return
	}

	fileSource := sourceFromPath(path)

	ast.Inspect(node, func(n ast.Node) bool {
		typeSpec, ok := n.(*ast.TypeSpec)
		if !ok {
			return true
		}
		structType, ok := typeSpec.Type.(*ast.StructType)
		if !ok {
			return true
		}

		table := &TableMetadata{
			StructName:     typeSpec.Name.Name,
			NormalizedName: normalizeEntityName(typeSpec.Name.Name),
			Source:         fileSource,
			Relations:      []*RelationNode{},
			Columns:        []ColumnInfo{},
			Operations:     []OperationInfo{},
		}

		for _, field := range structType.Fields.List {
			typeName, isCollection := resolveTypeInfo(field.Type)
			var vTag, dcTag, adTag, jsonTag string

			if field.Tag != nil {
				unquoted, err := strconv.Unquote(field.Tag.Value)
				if err == nil {
					// Professional-grade tag extraction using reflect.StructTag for multi-tag support
					tags := reflect.StructTag(unquoted)
					ormTag := tags.Get("orm")
					vTag = tags.Get("v")
					dcTag = tags.Get("dc")
					adTag = tags.Get("ad")
					jsonTag = parseJSONTag(tags.Get("json"))

					if strings.Contains(ormTag, "with:") {
						rel := parseWithTag(ormTag)
						if rel != nil {
							if len(field.Names) > 0 {
								rel.FieldName = field.Names[0].Name
							}
							rel.TargetStruct = typeName
							rel.IsCollection = isCollection
							rel.Validation = vTag
							rel.Description = dcTag
							table.Relations = append(table.Relations, rel)
							continue
						}
					}
				}
			}

			// Capture standard fields with validation and description metadata
			if len(field.Names) > 0 {
				table.Columns = append(table.Columns, ColumnInfo{
					Name:        field.Names[0].Name,
					JSONName:    jsonTag,
					Type:        typeName,
					Validation:  vTag,
					Description: dcTag,
					Additional:  adTag,
					IsArray:     isCollection,
					Source:      fileSource,
				})
			}
		}

		// Always track the table if it has fields or relations
		if len(table.Columns) > 0 || len(table.Relations) > 0 {
			putSchema(schema, table)
		}
		return true
	})
}

func parseJSONTag(tag string) string {
	if tag == "" {
		return ""
	}
	if tag == "-" {
		return ""
	}
	// json:"name,omitempty"
	if idx := strings.Index(tag, ","); idx != -1 {
		return tag[:idx]
	}
	return tag
}

// resolveTypeInfo unwraps pointers and slices to find the underlying struct name.
// e.g., []*UserDetail -> ("UserDetail", true)
// Handles nested pointers and preserves external package aliases.
func resolveTypeInfo(expr ast.Expr) (name string, isCollection bool) {
	for {
		switch t := expr.(type) {
		case *ast.StarExpr:
			expr = t.X // Peel pointer
		case *ast.ArrayType:
			isCollection = true
			expr = t.Elt // Peel slice
		case *ast.Ident:
			return t.Name, isCollection
		case *ast.SelectorExpr:
			// Preserve package namespace (e.g., "entity.User") for holistic generation.
			if x, ok := t.X.(*ast.Ident); ok {
				return x.Name + "." + t.Sel.Name, isCollection
			}
			return t.Sel.Name, isCollection
		default:
			return "Unknown", isCollection
		}
	}
}

// parseWithTag implements professional parsing for the orm:"with:..." syntax.
// with deterministic, allocation-friendly string splitting.
// Reference: mysql_z_unit_feature_with_test.go coverage
func parseWithTag(tag string) *RelationNode {
	// 1. Isolate the 'with' segment if multiple orm segments exist (comma separated)
	parts := strings.Split(tag, ",")
	var withPart string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if strings.HasPrefix(p, "with:") {
			withPart = strings.TrimPrefix(p, "with:")
			break
		}
	}

	if withPart == "" {
		return nil
	}

	rel := &RelationNode{}

	// Use SplitN for deterministic extraction of Target/Source keys.
	// Handles "uid=id", "uid = id", and implicit "uid" cases cleanly with trimming.
	kv := strings.SplitN(withPart, "=", 2)
	rel.TargetKey = strings.TrimSpace(kv[0])
	if len(kv) > 1 {
		rel.SourceKey = strings.TrimSpace(kv[1])
	} else {
		rel.SourceKey = "id" // GoFrame default
	}

	return rel
}

func printSchemaSummary(schema SchemaMap) {
	fmt.Println("\n--- 🏗️  HOLISTIC RELATION MAP ---")
	if len(schema) == 0 {
		fmt.Println("No 'with' associations found. Ensure pathing is correct.")
		return
	}

	metas := make([]*TableMetadata, 0, len(schema))
	for _, meta := range schema {
		metas = append(metas, meta)
	}
	sort.Slice(metas, func(i, j int) bool {
		if metas[i].NormalizedName == metas[j].NormalizedName {
			return metas[i].StructName < metas[j].StructName
		}
		return metas[i].NormalizedName < metas[j].NormalizedName
	})

	for _, meta := range metas {
		fmt.Printf("Struct: %s   (normalized: %s, source: %s)\n", meta.StructName, meta.NormalizedName, meta.Source)
		for _, rel := range meta.Relations {
			kind := "1:1"
			if rel.IsCollection {
				kind = "1:N"
			}
			fmt.Printf("  └─ [%s] %-12s -> %-15s (Map: %s=%s)\n",
				kind, rel.FieldName, rel.TargetStruct, rel.TargetKey, rel.SourceKey)
		}
	}
}

// generateERDiagram produces a Mermaid.js ER Diagram string from the parsed schema.
// Renders full entity attributes (columns) and relationship cardinality.
func generateERDiagram(schema SchemaMap) string {
	if len(schema) == 0 {
		return "erDiagram\n  %% No relations found"
	}

	var sb strings.Builder
	sb.WriteString("erDiagram\n")

	// 1. Define entities and their attributes
	metas := make([]*TableMetadata, 0, len(schema))
	for _, meta := range schema {
		metas = append(metas, meta)
	}
	sort.Slice(metas, func(i, j int) bool { return metas[i].StructName < metas[j].StructName })

	for _, meta := range metas {
		sb.WriteString(fmt.Sprintf("    %s {\n", meta.StructName))
		for _, col := range meta.Columns {
			// Mermaid types cannot contain special characters like '.' or '*'
			cleanType := strings.ReplaceAll(col.Type, ".", "_")
			sb.WriteString(fmt.Sprintf("        %s %s\n", cleanType, col.Name))
		}
		sb.WriteString("    }\n\n")
	}

	// 2. Define relationships with cardinality labels
	for _, meta := range metas {
		for _, rel := range meta.Relations {
			// Mermaid Cardinality: 1:1 is "||--||", 1:N is "||--o{"
			cardinality := "||--||"
			if rel.IsCollection {
				cardinality = "||--o{"
			}

			// Extract target name without package for Mermaid alias matching
			target := rel.TargetStruct
			if idx := strings.LastIndex(target, "."); idx != -1 {
				target = target[idx+1:]
			}

			label := fmt.Sprintf(`"%s (%s=%s)"`, rel.FieldName, rel.TargetKey, rel.SourceKey)
			sb.WriteString(fmt.Sprintf("    %s %s %s : %s\n", meta.StructName, cardinality, target, label))
		}
	}

	return sb.String()
}

/*
================================================================================
DEVELOPER MANUAL & DESIGN NOTES
================================================================================

1. WHY AST (STATIC ANALYSIS)?
   We avoid reflect.TypeOf() because it requires a running program. For code
   generators (UI scaffolds/API docs), we need to parse files directly from
   the file system.

2. GOFRAME RELATION PHILOSOPHY:
   This parser respects GF's Database-First approach. It assumes:
   - The 'do' objects are the source of truth for schema relations.
   - Relations are query-time (runtime) bindings, not hard DB constraints.

3. EDGE CASES HANDLED:
   - Spacing: "with:uid=id" vs "with: uid = id" are treated as equal.
   - Pointers: Supports *Struct and []*Struct (nested).
   - Implicit Keys: Handles `with:user_id` by defaulting source to `id`.
   - Complex Tags: Correctly extracts 'with' even if 'table' or 'where' tags exist.
   - Parse Errors: Skips files with errors, logs warnings.
   - Cross-Platform: Normalizes file paths for Windows compatibility.

4. NEXT STEPS FOR UI GENERATION:
   You can convert the `SchemaMap` to JSON or pass it to `text/template`.
   - 1:1 Relations -> Generate a Detail Card or a Join query.
   - 1:N Relations -> Generate a Sub-Table or a Tabbed view.
   - Mermaid visualization: Copy output to mermaid.live for architectural review.
   - Use NormalizedName field to group related structs (do + api req) logically.
================================================================================
*/

// ---- OpenAPI v3 (minimal) reader ------------------------------------------------

type openAPISpec struct {
	Openapi     string                 `json:"openapi"`
	Info        map[string]any         `json:"info"`
	Paths       map[string]openAPIPath `json:"paths"`
	Components  openAPIComponents      `json:"components"`
	Servers     []map[string]any       `json:"servers"`
	Security    []map[string]any       `json:"security"`
	Tags        []map[string]any       `json:"tags"`
	Extensions  map[string]any         `json:"-"`
	Raw         map[string]any         `json:"-"`
	ExternalDoc map[string]any         `json:"externalDocs"`
}

type openAPIComponents struct {
	Schemas map[string]*openAPISchema `json:"schemas"`
}

type openAPIPath map[string]*openAPIOperation

type openAPIOperation struct {
	OperationID string                      `json:"operationId"`
	Summary     string                      `json:"summary"`
	Tags        []string                    `json:"tags"`
	Parameters  []map[string]any            `json:"parameters"`
	RequestBody *openAPIRequestBody         `json:"requestBody"`
	Responses   map[string]*openAPIResponse `json:"responses"`
}

type openAPIRequestBody struct {
	Content map[string]*openAPIMediaType `json:"content"`
}

type openAPIResponse struct {
	Description string                       `json:"description"`
	Content     map[string]*openAPIMediaType `json:"content"`
}

type openAPIMediaType struct {
	Schema *openAPISchema `json:"schema"`
}

type openAPISchema struct {
	Ref                  string                    `json:"$ref"`
	Type                 string                    `json:"type"`
	Format               string                    `json:"format"`
	Description          string                    `json:"description"`
	Properties           map[string]*openAPISchema `json:"properties"`
	Items                *openAPISchema            `json:"items"`
	Required             []string                  `json:"required"`
	Enum                 []any                     `json:"enum"`
	Nullable             bool                      `json:"nullable"`
	MinLength            *int                      `json:"minLength"`
	MaxLength            *int                      `json:"maxLength"`
	Minimum              *float64                  `json:"minimum"`
	Maximum              *float64                  `json:"maximum"`
	Pattern              string                    `json:"pattern"`
	AllOf                []*openAPISchema          `json:"allOf"`
	OneOf                []*openAPISchema          `json:"oneOf"`
	AnyOf                []*openAPISchema          `json:"anyOf"`
	AdditionalProperties any                       `json:"additionalProperties"`
}

func parseOpenAPIFile(path string) (SchemaMap, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	b, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}

	var spec openAPISpec
	if err := json.Unmarshal(b, &spec); err != nil {
		return nil, err
	}

	out := make(SchemaMap)

	// 1) Component schemas as entity candidates
	for schemaName, schema := range spec.Components.Schemas {
		meta := openAPISchemaToTableMetadata(&spec, schemaName, schema)
		putSchema(out, meta)
	}

	// 2) Operations associated to entities by request/response/tags/path heuristics
	opsByNorm := make(map[string][]OperationInfo)
	for p, methods := range spec.Paths {
		for method, op := range methods {
			if op == nil {
				continue
			}
			oi := openAPIOperationInfo(&spec, p, strings.ToUpper(method), op)
			norm := normalizeEntityName(inferEntityNameForOperation(oi))
			if norm == "" {
				continue
			}
			opsByNorm[norm] = append(opsByNorm[norm], oi)
		}
	}

	for _, meta := range out {
		if ops, ok := opsByNorm[meta.NormalizedName]; ok {
			meta.Operations = append(meta.Operations, ops...)
		}
	}

	return out, nil
}

func openAPISchemaToTableMetadata(spec *openAPISpec, schemaName string, schema *openAPISchema) *TableMetadata {
	props := make(map[string]*openAPISchema)
	required := make(map[string]bool)

	visited := make(map[string]bool)
	collectOpenAPIObject(spec, schema, visited, props, required)

	cols := make([]ColumnInfo, 0, len(props))
	keys := make([]string, 0, len(props))
	for k := range props {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, propName := range keys {
		ps := props[propName]
		typeName, isArray, refName := openAPITypeName(spec, ps)
		c := openAPIConstraintsForSchema(ps)
		if required[propName] {
			if c == nil {
				c = &FieldConstraints{}
			}
			c.Required = true
		}

		cols = append(cols, ColumnInfo{
			Name:        propName,
			JSONName:    propName,
			Type:        typeName,
			Description: ps.Description,
			Constraints: c,
			Ref:         refName,
			IsArray:     isArray,
			Source:      "openapi",
		})
	}

	return &TableMetadata{
		StructName:     schemaName,
		NormalizedName: normalizeEntityName(schemaName),
		Source:         "openapi",
		Columns:        cols,
		Relations:      []*RelationNode{},
		Operations:     []OperationInfo{},
	}
}

func collectOpenAPIObject(spec *openAPISpec, s *openAPISchema, visited map[string]bool, props map[string]*openAPISchema, required map[string]bool) {
	if s == nil {
		return
	}
	if s.Ref != "" {
		name := openAPIRefName(s.Ref)
		if name == "" {
			return
		}
		if visited[name] {
			return
		}
		visited[name] = true
		collectOpenAPIObject(spec, spec.Components.Schemas[name], visited, props, required)
		return
	}

	for _, r := range s.Required {
		required[r] = true
	}

	// allOf is used heavily by real-world generators for schema composition.
	for _, sub := range s.AllOf {
		collectOpenAPIObject(spec, sub, visited, props, required)
	}

	// oneOf/anyOf are preserved as a first-class schema feature in OpenAPI;
	// object property extraction selects a deterministic branch for metadata purposes.
	if len(s.OneOf) > 0 {
		collectOpenAPIObject(spec, s.OneOf[0], visited, props, required)
	}
	if len(s.AnyOf) > 0 {
		collectOpenAPIObject(spec, s.AnyOf[0], visited, props, required)
	}

	for k, v := range s.Properties {
		props[k] = v
	}
}

func openAPITypeName(spec *openAPISpec, s *openAPISchema) (typeName string, isArray bool, refName string) {
	if s == nil {
		return "Unknown", false, ""
	}
	if s.Ref != "" {
		refName = openAPIRefName(s.Ref)
		if refName == "" {
			return "Unknown", false, ""
		}
		return refName, false, refName
	}

	switch s.Type {
	case "array":
		itemType, _, itemRef := openAPITypeName(spec, s.Items)
		return "[]" + itemType, true, itemRef
	case "object":
		// Component references are handled via $ref; inline objects remain explicit.
		return "object", false, ""
	case "string":
		return "string", false, ""
	case "integer":
		if s.Format == "int64" {
			return "int64", false, ""
		}
		return "int", false, ""
	case "number":
		if s.Format == "float" {
			return "float32", false, ""
		}
		return "float64", false, ""
	case "boolean":
		return "bool", false, ""
	default:
		// OpenAPI allows schemas without explicit "type" when using composition.
		if len(s.AllOf) > 0 {
			return "object", false, ""
		}
		return "Unknown", false, ""
	}
}

func openAPIConstraintsForSchema(s *openAPISchema) *FieldConstraints {
	if s == nil {
		return nil
	}

	var enumStrings []string
	if len(s.Enum) > 0 {
		enumStrings = make([]string, 0, len(s.Enum))
		for _, v := range s.Enum {
			enumStrings = append(enumStrings, fmt.Sprint(v))
		}
	}

	c := &FieldConstraints{
		Nullable:  s.Nullable,
		MinLength: s.MinLength,
		MaxLength: s.MaxLength,
		Minimum:   s.Minimum,
		Maximum:   s.Maximum,
		Pattern:   s.Pattern,
		Format:    s.Format,
		Enum:      enumStrings,
	}

	if constraintsEmpty(c) {
		return nil
	}
	return c
}

func constraintsEmpty(c *FieldConstraints) bool {
	if c == nil {
		return true
	}
	if c.Required || c.Nullable {
		return false
	}
	if c.MinLength != nil || c.MaxLength != nil || c.Minimum != nil || c.Maximum != nil {
		return false
	}
	if c.Pattern != "" || c.Format != "" {
		return false
	}
	if len(c.Enum) > 0 {
		return false
	}
	return true
}

func openAPIRefName(ref string) string {
	// "#/components/schemas/SomeName"
	const pfx = "#/components/schemas/"
	if strings.HasPrefix(ref, pfx) {
		return strings.TrimPrefix(ref, pfx)
	}
	if idx := strings.LastIndex(ref, "/"); idx != -1 {
		return ref[idx+1:]
	}
	return ""
}

func openAPIOperationInfo(spec *openAPISpec, path, method string, op *openAPIOperation) OperationInfo {
	reqSchema := pickOpenAPISchemaRefName(op.RequestBody)
	respSchema := pickOpenAPIResponseSchemaRefName(op.Responses)

	return OperationInfo{
		Method:         method,
		Path:           path,
		OperationID:    op.OperationID,
		Summary:        op.Summary,
		Tags:           append([]string(nil), op.Tags...),
		RequestSchema:  reqSchema,
		ResponseSchema: respSchema,
		Source:         "openapi",
	}
}

func pickOpenAPISchemaRefName(rb *openAPIRequestBody) string {
	if rb == nil || len(rb.Content) == 0 {
		return ""
	}
	s := pickJSONMediaSchema(rb.Content)
	return openAPISchemaRefOrItemRef(s)
}

func pickOpenAPIResponseSchemaRefName(resps map[string]*openAPIResponse) string {
	if len(resps) == 0 {
		return ""
	}
	// Prefer success codes, then fallback deterministically.
	for _, code := range []string{"200", "201", "202", "204"} {
		if r := resps[code]; r != nil {
			s := pickJSONMediaSchema(r.Content)
			return openAPISchemaRefOrItemRef(s)
		}
	}
	if r := resps["default"]; r != nil {
		s := pickJSONMediaSchema(r.Content)
		return openAPISchemaRefOrItemRef(s)
	}

	codes := make([]string, 0, len(resps))
	for c := range resps {
		codes = append(codes, c)
	}
	sort.Strings(codes)
	r := resps[codes[0]]
	if r == nil {
		return ""
	}
	s := pickJSONMediaSchema(r.Content)
	return openAPISchemaRefOrItemRef(s)
}

func pickJSONMediaSchema(content map[string]*openAPIMediaType) *openAPISchema {
	if len(content) == 0 {
		return nil
	}
	if mt := content["application/json"]; mt != nil {
		return mt.Schema
	}
	if mt := content["application/ld+json"]; mt != nil {
		return mt.Schema
	}
	keys := make([]string, 0, len(content))
	for k := range content {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return content[keys[0]].Schema
}

func openAPISchemaRefOrItemRef(s *openAPISchema) string {
	if s == nil {
		return ""
	}
	if s.Ref != "" {
		return openAPIRefName(s.Ref)
	}
	if s.Type == "array" && s.Items != nil && s.Items.Ref != "" {
		return openAPIRefName(s.Items.Ref)
	}
	return ""
}

func inferEntityNameForOperation(oi OperationInfo) string {
	if oi.RequestSchema != "" {
		return oi.RequestSchema
	}
	if oi.ResponseSchema != "" {
		return oi.ResponseSchema
	}
	if len(oi.Tags) > 0 && oi.Tags[0] != "" {
		return oi.Tags[0]
	}
	return entityFromPathHeuristic(oi.Path)
}

func entityFromPathHeuristic(p string) string {
	// "/users/{id}" -> "users" -> "user"
	trim := strings.Trim(p, "/")
	if trim == "" {
		return ""
	}
	parts := strings.Split(trim, "/")
	// Drop leading common API prefixes
	if len(parts) > 0 && (parts[0] == "api" || strings.HasPrefix(parts[0], "v")) {
		parts = parts[1:]
	}
	if len(parts) == 0 {
		return ""
	}
	last := parts[len(parts)-1]
	if strings.HasPrefix(last, "{") && len(parts) > 1 {
		last = parts[len(parts)-2]
	}
	last = strings.TrimSpace(last)
	last = strings.TrimSuffix(last, "s")
	// Path segments are typically lowercase; normalizeEntityName expects an identifier-like string.
	if last == "" {
		return ""
	}
	return strings.ToUpper(last[:1]) + last[1:]
}

// ---- Consolidation ------------------------------------------------------------

func consolidateByNormalizedName(schema SchemaMap) ConsolidatedSchema {
	entities := make(map[string]*TableMetadata) // key = NormalizedName

	for _, entry := range schema {
		norm := entry.NormalizedName
		if norm == "" {
			norm = normalizeEntityName(entry.StructName)
		}
		if norm == "" {
			continue
		}

		if existing, ok := entities[norm]; ok {
			mergeTableMetadata(existing, entry)
		} else {
			entities[norm] = cloneTableMetadata(entry)
		}
	}

	list := make([]*TableMetadata, 0, len(entities))
	for _, e := range entities {
		stabilizeTableMetadata(e)
		list = append(list, e)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].NormalizedName < list[j].NormalizedName })

	return ConsolidatedSchema{
		Entities:    entities,
		EntityList:  list,
		GeneratedBy: "schema-architect",
	}
}

func cloneTableMetadata(in *TableMetadata) *TableMetadata {
	if in == nil {
		return nil
	}
	out := &TableMetadata{
		StructName:     in.StructName,
		NormalizedName: in.NormalizedName,
		Source:         in.Source,
	}
	if len(in.Columns) > 0 {
		out.Columns = append([]ColumnInfo(nil), in.Columns...)
	}
	if len(in.Relations) > 0 {
		out.Relations = append([]*RelationNode(nil), in.Relations...)
	}
	if len(in.Operations) > 0 {
		out.Operations = append([]OperationInfo(nil), in.Operations...)
	}
	return out
}

func mergeTableMetadata(dst, src *TableMetadata) {
	if dst == nil || src == nil {
		return
	}

	dst.Source = "merged"

	mergeColumns(&dst.Columns, src.Columns)
	mergeRelations(&dst.Relations, src.Relations)
	mergeOperations(&dst.Operations, src.Operations)

	// Prefer the most specific struct name when OpenAPI provides canonical schema names.
	if dst.StructName == "" || (dst.Source == "merged" && src.Source == "openapi") {
		if src.StructName != "" {
			dst.StructName = src.StructName
		}
	}
}

func mergeColumns(dst *[]ColumnInfo, src []ColumnInfo) {
	if dst == nil {
		return
	}

	index := make(map[string]int, len(*dst))
	for i := range *dst {
		index[columnKey((*dst)[i])] = i
	}

	for _, c := range src {
		k := columnKey(c)
		if k == "" {
			continue
		}

		if i, ok := index[k]; ok {
			(*dst)[i] = mergeColumn((*dst)[i], c)
		} else {
			*dst = append(*dst, c)
			index[k] = len(*dst) - 1
		}
	}
}

func mergeColumn(a, b ColumnInfo) ColumnInfo {
	// Field identity is maintained by the caller; this function selects richer metadata.
	out := a

	if out.Name == "" {
		out.Name = b.Name
	}
	if out.JSONName == "" {
		out.JSONName = b.JSONName
	}
	if out.Type == "" || out.Type == "Unknown" {
		if b.Type != "" {
			out.Type = b.Type
		}
	}
	if out.Description == "" {
		out.Description = b.Description
	}
	if out.Validation == "" {
		out.Validation = b.Validation
	}
	if out.Additional == "" {
		out.Additional = b.Additional
	}
	if out.Ref == "" {
		out.Ref = b.Ref
	}
	out.IsArray = out.IsArray || b.IsArray

	out.Constraints = mergeConstraints(out.Constraints, b.Constraints)

	if out.Source == "" {
		out.Source = b.Source
	}
	return out
}

func mergeConstraints(a, b *FieldConstraints) *FieldConstraints {
	if a == nil && b == nil {
		return nil
	}
	if a == nil {
		cp := *b
		return &cp
	}
	if b == nil {
		return a
	}

	// Deterministic union for generators: preserve all information, prefer "stricter" bounds.
	out := *a

	out.Required = out.Required || b.Required
	out.Nullable = out.Nullable || b.Nullable

	out.MinLength = pickIntPtrMax(out.MinLength, b.MinLength)
	out.MaxLength = pickIntPtrMin(out.MaxLength, b.MaxLength)

	out.Minimum = pickFloatPtrMax(out.Minimum, b.Minimum)
	out.Maximum = pickFloatPtrMin(out.Maximum, b.Maximum)

	if out.Pattern == "" {
		out.Pattern = b.Pattern
	}
	if out.Format == "" {
		out.Format = b.Format
	}
	if len(out.Enum) == 0 && len(b.Enum) > 0 {
		out.Enum = append([]string(nil), b.Enum...)
	}

	if constraintsEmpty(&out) {
		return nil
	}
	return &out
}

func pickIntPtrMax(a, b *int) *int {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	if *b > *a {
		return b
	}
	return a
}

func pickIntPtrMin(a, b *int) *int {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	if *b < *a {
		return b
	}
	return a
}

func pickFloatPtrMax(a, b *float64) *float64 {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	if *b > *a {
		return b
	}
	return a
}

func pickFloatPtrMin(a, b *float64) *float64 {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	if *b < *a {
		return b
	}
	return a
}

func mergeRelations(dst *[]*RelationNode, src []*RelationNode) {
	if dst == nil {
		return
	}
	seen := make(map[string]bool, len(*dst))
	for _, r := range *dst {
		seen[relationKey(r)] = true
	}
	for _, r := range src {
		k := relationKey(r)
		if k == "" {
			continue
		}
		if seen[k] {
			continue
		}
		*dst = append(*dst, r)
		seen[k] = true
	}
}

func mergeOperations(dst *[]OperationInfo, src []OperationInfo) {
	if dst == nil {
		return
	}
	seen := make(map[string]bool, len(*dst))
	for _, op := range *dst {
		seen[operationKey(op)] = true
	}
	for _, op := range src {
		k := operationKey(op)
		if k == "" {
			continue
		}
		if seen[k] {
			continue
		}
		*dst = append(*dst, op)
		seen[k] = true
	}
}

func stabilizeTableMetadata(t *TableMetadata) {
	if t == nil {
		return
	}
	sort.Slice(t.Columns, func(i, j int) bool {
		ai := t.Columns[i].JSONName
		aj := t.Columns[j].JSONName
		if ai == "" {
			ai = t.Columns[i].Name
		}
		if aj == "" {
			aj = t.Columns[j].Name
		}
		return ai < aj
	})
	sort.Slice(t.Operations, func(i, j int) bool {
		if t.Operations[i].Path == t.Operations[j].Path {
			return t.Operations[i].Method < t.Operations[j].Method
		}
		return t.Operations[i].Path < t.Operations[j].Path
	})
}

func columnKey(c ColumnInfo) string {
	s := c.JSONName
	if s == "" {
		s = c.Name
	}
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "_", "")
	s = strings.ReplaceAll(s, "-", "")
	return s
}

func relationKey(r *RelationNode) string {
	if r == nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(r.FieldName)) + "|" +
		strings.ToLower(strings.TrimSpace(r.TargetStruct)) + "|" +
		strings.ToLower(strings.TrimSpace(r.TargetKey)) + "|" +
		strings.ToLower(strings.TrimSpace(r.SourceKey))
}

func operationKey(op OperationInfo) string {
	return op.Method + "|" + op.Path + "|" + op.OperationID
}

// ---- JSON output --------------------------------------------------------------

func writeJSONFile(path string, v any) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
