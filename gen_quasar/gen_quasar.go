package main

import (
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"
	"unicode"
)

/*
================================================================================
QUASAR CRUD GENERATOR
================================================================================
Reads the consolidated schema (schema.logical.json) produced by the schema parser
and generates a production-ready Quasar CRUD UI scaffold:

  Per-entity:  IndexPage.vue, FormDialog.vue, DetailPage.vue, use{Entity}.ts
  Shared:      SubTableCrud.vue, PivotSelect.vue
  Global:      API client, router, validation utils, Hydra/IRI helpers,
               Zod-to-Quasar bridge, Orval config (dual vue-query + zod)

Template engine: Go text/template with [[ ]] delimiters to avoid Vue {{ }} conflict.
All templates are embedded as string constants for single-binary portability.

OUTPUT STRUCTURE:
  src-gen/
    api/client.ts
    components/SubTableCrud.vue       Reusable 1:N sub-table with inline CRUD
    components/PivotSelect.vue        Reusable M2M chip-based multi-select
    composables/use{Entity}.ts
    pages/{entity}/IndexPage.vue
    pages/{entity}/FormDialog.vue
    pages/{entity}/DetailPage.vue
    router/generated-routes.ts
    utils/validation.ts
    utils/hydra.ts
    utils/zod-to-quasar.ts
    orval.config.ts
================================================================================
*/

// ======================== Schema Types (match parser JSON output) ========================

type ConsolidatedSchema struct {
	Entities    map[string]*TableMetadata `json:"entities"`
	EntityList  []*TableMetadata          `json:"entity_list"`
	GeneratedBy string                    `json:"generated_by"`
}

type TableMetadata struct {
	StructName     string          `json:"StructName"`
	NormalizedName string          `json:"NormalizedName"`
	Source         string          `json:"Source"`
	Columns        []ColumnInfo    `json:"Columns"`
	Relations      []*RelationNode `json:"Relations"`
	Operations     []OperationInfo `json:"Operations"`
}

type ColumnInfo struct {
	Name        string            `json:"Name"`
	JSONName    string            `json:"JSONName"`
	Type        string            `json:"Type"`
	Validation  string            `json:"Validation"`
	Description string            `json:"Description"`
	Additional  string            `json:"Additional"`
	Constraints *FieldConstraints `json:"Constraints"`
	Ref         string            `json:"Ref"`
	IsArray     bool              `json:"IsArray"`
	Source      string            `json:"Source"`
}

type FieldConstraints struct {
	Required  bool     `json:"Required"`
	Nullable  bool     `json:"Nullable"`
	MinLength *int     `json:"MinLength"`
	MaxLength *int     `json:"MaxLength"`
	Minimum   *float64 `json:"Minimum"`
	Maximum   *float64 `json:"Maximum"`
	Pattern   string   `json:"Pattern"`
	Format    string   `json:"Format"`
	Enum      []string `json:"Enum"`
}

type RelationNode struct {
	FieldName    string `json:"field_name"`
	TargetStruct string `json:"target_struct"`
	IsCollection bool   `json:"is_collection"`
	TargetKey    string `json:"target_key"`
	SourceKey    string `json:"source_key"`
	Validation   string `json:"validation"`
	Description  string `json:"description"`
}

type OperationInfo struct {
	Method         string   `json:"method"`
	Path           string   `json:"path"`
	OperationID    string   `json:"operation_id"`
	Summary        string   `json:"summary"`
	Tags           []string `json:"tags"`
	RequestSchema  string   `json:"request_schema"`
	ResponseSchema string   `json:"response_schema"`
	Source         string   `json:"source"`
}

// ======================== View Model Types ========================

type GlobalView struct {
	Entities   []EntityView
	APIBaseURL string
	OpenAPIURL string
}

type EntityView struct {
	Name            string
	NameLower       string
	NameKebab       string
	NameSnake       string
	NameHuman       string
	NamePlural      string
	NamePluralLower string
	NamePluralKebab string
	NamePluralHuman string
	APIBasePath     string

	PrimaryKey   string
	DisplayField string

	AllColumns  []ColumnView
	ListColumns []ColumnView
	FormFields  []ColumnView

	TableRelations  []RelationView
	SelectRelations []RelationView

	HasFileUpload    bool
	HasEnum          bool
	HasRelations     bool
	HasPivot         bool // M2M array-of-ID fields present
	HasNestedObjects bool // Embedded object/JSON fields present
	Operations       []OperationInfo
	CreateSchema     string
	UpdateSchema     string
	ZodImportPath    string
}

type ColumnView struct {
	Name      string
	JSONName  string
	Label     string
	GoType    string
	TSType    string
	Component string
	InputType string

	IsPrimaryKey   bool
	IsTextarea     bool
	IsFile         bool
	IsEnum         bool
	IsRelation     bool
	IsPivot        bool // M2M: array of scalar IDs
	IsNestedObject bool // Embedded object or array of objects
	IsArray        bool
	Sortable       bool
	Align          string

	RelationEntity      string
	RelationEntityLower string
	RelationEntityKebab string
	RelationAPIPath     string

	EnumOptions string
	QuasarRules string
	Required    bool
}

type RelationView struct {
	FieldName          string
	TargetEntity       string
	TargetLower        string
	TargetKebab        string
	TargetPlural       string
	TargetPluralKebab  string
	TargetAPIPath      string // Full API path for fetching related items
	TargetKey          string
	SourceKey          string
	IsCollection       bool
	Description        string
	TargetCreateSchema string
	TargetUpdateSchema string
	ZodImportPath      string
}

// ======================== Template Constants — Global ========================

//go:embed tplAPIClient.ts
var tplAPIClient string

//go:embed tplRouter.ts
var tplRouter string

//go:embed tplValidation.ts
var tplValidation string

//go:embed tplHydra.ts
var tplHydra string

//go:embed tplZodBridge.ts
var tplZodBridge string

//go:embed tplOrvalConfig.ts
var tplOrvalConfig string

// ======================== Template Constants — Shared Components ========================

// SubTableCrud provides embedded 1:N relation CRUD inside any detail page.
// Dynamic columns are derived from response data, so no schema lookup is needed.
//
//go:embed tplSubTableCrud.vue
var tplSubTableCrud string

// PivotSelect provides a chip-based multi-select for M2M relationships.
// Options are fetched from the target entity endpoint with type-ahead filtering.
//
//go:embed tplPivotSelect.vue
var tplPivotSelect string

// ======================== Template Constants — Per-Entity ========================

//go:embed tplIndexPage.vue
var tplIndexPage string

//go:embed tplFormDialog.vue
var tplFormDialog string

//go:embed tplDetailPage.vue
var tplDetailPage string

//go:embed tplComposable.ts
var tplComposable string

// ======================== Main ========================

func main() {
	var (
		schemaPath = flag.String("schema", "schema.logical.json", "Path to consolidated schema JSON")
		outDir     = flag.String("out", "./src-gen", "Output directory for generated files")
		apiBase    = flag.String("api-base", "/api", "API base URL prefix for composables")
		openAPIURL = flag.String("openapi-url", "http://localhost:8000/api.json", "OpenAPI spec URL for Orval")
	)
	flag.Parse()

	schema, err := loadSchema(*schemaPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to load schema: %v\n", err)
		os.Exit(1)
	}

	var entities []EntityView
	seen := make(map[string]bool)

	sources := schema.EntityList
	if len(sources) == 0 {
		for _, m := range schema.Entities {
			sources = append(sources, m)
		}
	}

	for _, meta := range sources {
		if meta == nil || meta.NormalizedName == "" {
			continue
		}
		if seen[meta.NormalizedName] {
			continue
		}
		seen[meta.NormalizedName] = true
		if len(meta.Columns) == 0 && len(meta.Relations) == 0 {
			continue
		}
		entities = append(entities, buildEntityView(meta, *apiBase, schema))
	}

	sort.Slice(entities, func(i, j int) bool { return entities[i].Name < entities[j].Name })

	if len(entities) == 0 {
		fmt.Println("⚠️  No entities found in schema. Nothing to generate.")
		return
	}

	global := GlobalView{
		Entities:   entities,
		APIBaseURL: *apiBase,
		OpenAPIURL: *openAPIURL,
	}

	funcMap := template.FuncMap{
		"bt": func() string { return "`" },
	}
	templates := template.New("root").Delims("[[", "]]").Funcs(funcMap)

	tplDefs := map[string]string{
		"api-client":     tplAPIClient,
		"router":         tplRouter,
		"validation":     tplValidation,
		"hydra":          tplHydra,
		"zod-bridge":     tplZodBridge,
		"orval":          tplOrvalConfig,
		"sub-table-crud": tplSubTableCrud,
		"pivot-select":   tplPivotSelect,
		"index-page":     tplIndexPage,
		"form-dialog":    tplFormDialog,
		"detail-page":    tplDetailPage,
		"composable":     tplComposable,
	}
	for name, content := range tplDefs {
		if _, err := templates.New(name).Parse(content); err != nil {
			fmt.Fprintf(os.Stderr, "❌ Template parse error (%s): %v\n", name, err)
			os.Exit(1)
		}
	}

	// Global scaffolding files
	globalFiles := []struct {
		tpl, path string
		data      any
	}{
		{"api-client", filepath.Join(*outDir, "api", "client.ts"), global},
		{"router", filepath.Join(*outDir, "router", "generated-routes.ts"), global},
		{"validation", filepath.Join(*outDir, "utils", "validation.ts"), nil},
		{"hydra", filepath.Join(*outDir, "utils", "hydra.ts"), nil},
		{"zod-bridge", filepath.Join(*outDir, "utils", "zod-to-quasar.ts"), nil},
		{"orval", filepath.Join(*outDir, "orval.config.ts"), global},
	}
	for _, gf := range globalFiles {
		if err := renderToFile(templates, gf.tpl, gf.path, gf.data); err != nil {
			fmt.Fprintf(os.Stderr, "❌ %v\n", err)
		}
	}

	// Shared reusable components (no template variables)
	sharedFiles := []struct{ tpl, path string }{
		{"sub-table-crud", filepath.Join(*outDir, "components", "SubTableCrud.vue")},
		{"pivot-select", filepath.Join(*outDir, "components", "PivotSelect.vue")},
	}
	for _, sf := range sharedFiles {
		if err := renderToFile(templates, sf.tpl, sf.path, nil); err != nil {
			fmt.Fprintf(os.Stderr, "❌ %v\n", err)
		}
	}

	// Per-entity files
	for _, ev := range entities {
		entityFiles := []struct{ tpl, path string }{
			{"index-page", filepath.Join(*outDir, "pages", ev.NameKebab, "IndexPage.vue")},
			{"form-dialog", filepath.Join(*outDir, "pages", ev.NameKebab, "FormDialog.vue")},
			{"detail-page", filepath.Join(*outDir, "pages", ev.NameKebab, "DetailPage.vue")},
			{"composable", filepath.Join(*outDir, "composables", "use"+ev.Name+".ts")},
		}
		for _, ef := range entityFiles {
			if err := renderToFile(templates, ef.tpl, ef.path, ev); err != nil {
				fmt.Fprintf(os.Stderr, "❌ %v\n", err)
			}
		}
	}

	fmt.Printf("✅ Generated Quasar CRUD UI for %d entities in %s\n", len(entities), *outDir)
}

// ======================== Schema Loading ========================

func loadSchema(path string) (*ConsolidatedSchema, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var cs ConsolidatedSchema
	if err := json.Unmarshal(data, &cs); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &cs, nil
}

// ======================== View Model Builders ========================

func buildEntityView(meta *TableMetadata, apiBase string, schema *ConsolidatedSchema) EntityView {
	name := toPascal(meta.NormalizedName)
	plural := toPlural(name)

	ev := EntityView{
		Name:            name,
		NameLower:       toCamel(name),
		NameKebab:       toKebab(name),
		NameSnake:       toSnake(name),
		NameHuman:       toHuman(name),
		NamePlural:      plural,
		NamePluralLower: toCamel(plural),
		NamePluralKebab: toKebab(plural),
		NamePluralHuman: toHuman(plural),
		APIBasePath:     apiBase + "/" + toKebab(plural),
		Operations:      meta.Operations,
	}

	// Heuristic: Link Zod schemas from OpenAPI operations
	for _, op := range meta.Operations {
		// POST to collection is usually Create
		if op.Method == "POST" && ev.CreateSchema == "" && len(op.Tags) > 0 {
			if op.RequestSchema != "" {
				tag := op.Tags[0]
				pascalTag := toPascal(tag)
				kebabTag := toKebab(tag)
				ev.CreateSchema = "Post" + pascalTag + "Body"
				if ev.ZodImportPath == "" {
					ev.ZodImportPath = "../../api/gen/zod/" + kebabTag + "/" + kebabTag
				}
			}
		}
		// PUT/PATCH to item is usually Update
		if (op.Method == "PUT" || op.Method == "PATCH") && ev.UpdateSchema == "" && len(op.Tags) > 0 {
			if op.RequestSchema != "" {
				tag := op.Tags[0]
				pascalTag := toPascal(tag)
				kebabTag := toKebab(tag)
				ev.UpdateSchema = strings.Title(op.Method) + pascalTag + "IdBody"
				if ev.ZodImportPath == "" {
					ev.ZodImportPath = "../../api/gen/zod/" + kebabTag + "/" + kebabTag
				}
			}
		}
	}

	allCols := make([]ColumnView, 0, len(meta.Columns))
	for _, col := range meta.Columns {
		allCols = append(allCols, buildColumnView(col, apiBase))
	}
	ev.AllColumns = allCols

	ev.PrimaryKey = detectPrimaryKey(allCols)
	ev.DisplayField = detectDisplayField(allCols, ev.PrimaryKey)

	autoTimestamps := map[string]bool{
		"created_at": true, "updated_at": true, "deleted_at": true,
		"createdAt": true, "updatedAt": true, "deletedAt": true,
		"create_at": true, "update_at": true, "delete_at": true,
	}
	for _, cv := range allCols {
		if !cv.IsTextarea && !cv.IsFile {
			ev.ListColumns = append(ev.ListColumns, cv)
		}
		if !cv.IsPrimaryKey && !autoTimestamps[cv.JSONName] {
			ev.FormFields = append(ev.FormFields, cv)
		}
		if cv.IsFile {
			ev.HasFileUpload = true
		}
		if cv.IsEnum {
			ev.HasEnum = true
		}
		if cv.IsRelation || cv.IsPivot {
			ev.HasRelations = true
		}
		if cv.IsPivot {
			ev.HasPivot = true
		}
		if cv.IsNestedObject {
			ev.HasNestedObjects = true
		}
	}

	for _, rel := range meta.Relations {
		rv := buildRelationView(rel, apiBase, schema)
		if rel.IsCollection {
			ev.TableRelations = append(ev.TableRelations, rv)
		} else {
			ev.SelectRelations = append(ev.SelectRelations, rv)
		}
	}
	if len(ev.TableRelations) > 0 || len(ev.SelectRelations) > 0 {
		ev.HasRelations = true
	}

	return ev
}

// buildColumnView resolves a single schema column into template-ready metadata,
// mapping Go types to Quasar components, detecting files/enums/relations/pivots/nested,
// and pre-computing validation rules.
func buildColumnView(col ColumnInfo, apiBase string) ColumnView {
	jsonName := col.JSONName
	if jsonName == "" {
		jsonName = col.Name // Preserve GoFrame's actual field name
	}

	cv := ColumnView{
		Name:      col.Name,
		JSONName:  jsonName,
		Label:     toHuman(col.Name),
		GoType:    col.Type,
		IsArray:   col.IsArray,
		Sortable:  true,
		Align:     "left",
		Component: "q-input",
		InputType: "text",
		TSType:    "string",
	}

	if col.Constraints != nil {
		cv.Required = col.Constraints.Required
		if col.Constraints.Format != "" {
			cv.InputType = mapFormatToInputType(col.Constraints.Format)
		}
	}

	lowerJSON := strings.ToLower(jsonName)

	// Primary key detection
	if lowerJSON == "id" {
		cv.IsPrimaryKey = true
	}

	// File upload detection (by format or naming convention)
	if col.Constraints != nil && col.Constraints.Format == "binary" {
		cv.IsFile = true
	}
	if !cv.IsFile {
		fileKeywords := []string{"file", "image", "avatar", "photo", "attachment", "document", "upload", "thumbnail", "cover"}
		nameLower := strings.ToLower(col.Name)
		for _, kw := range fileKeywords {
			if strings.Contains(nameLower, kw) {
				cv.IsFile = true
				break
			}
		}
	}
	if cv.IsFile {
		cv.Component = "q-uploader"
		cv.TSType = "string"
		cv.Sortable = false
		cv.QuasarRules = buildQuasarRules(cv, col)
		return cv
	}

	// Enum detection
	if col.Constraints != nil && len(col.Constraints.Enum) > 0 {
		cv.IsEnum = true
		cv.Component = "q-select"
		cv.EnumOptions = formatEnumOptions(col.Constraints.Enum)
		cv.QuasarRules = buildQuasarRules(cv, col)
		return cv
	}

	// Pivot (M2M) detection: array of scalar IDs (e.g., role_ids: []int)
	if col.IsArray && !cv.IsPrimaryKey {
		typeLower := strings.ToLower(col.Type)
		if strings.Contains(typeLower, "int") || strings.Contains(typeLower, "uint") || strings.Contains(typeLower, "string") {
			lName := strings.ToLower(col.Name)
			if strings.HasSuffix(lName, "ids") || strings.HasSuffix(lName, "_ids") {
				cv.IsPivot = true
				cv.Component = "pivot-select"
				cv.TSType = "any[]"
				cv.Sortable = false
				rawEntity := strings.TrimSuffix(strings.TrimSuffix(lName, "_ids"), "ids")
				rawEntity = strings.TrimRight(rawEntity, "_")
				if rawEntity != "" {
					setRelationFields(&cv, normalizeEntityName(rawEntity), apiBase)
				}
				cv.QuasarRules = buildQuasarRules(cv, col)
				return cv
			}
		}
	}

	// Nested object detection: $ref without FK naming or object type without relation
	if !cv.IsPrimaryKey && !cv.IsFile && !cv.IsEnum && !cv.IsPivot {
		if col.Ref != "" {
			// Only treat as relation if field name follows FK naming convention
			hasFKSuffix := strings.HasSuffix(lowerJSON, "_id") ||
				(lowerJSON != "id" && len(lowerJSON) > 2 && strings.HasSuffix(lowerJSON, "id"))
			if !hasFKSuffix {
				// $ref without FK suffix → embedded/nested object
				cv.IsNestedObject = true
				cv.TSType = "any"
				cv.Sortable = false
				cv.QuasarRules = buildQuasarRules(cv, col)
				return cv
			}
		}
		typeLower := strings.ToLower(col.Type)
		if typeLower == "object" || (col.IsArray && col.Ref != "") {
			cv.IsNestedObject = true
			cv.TSType = "any"
			cv.Sortable = false
			cv.QuasarRules = buildQuasarRules(cv, col)
			return cv
		}
	}

	// Relation detection (by $ref with FK suffix, or naming convention)
	if !cv.IsPrimaryKey && !cv.IsFile && !cv.IsEnum && !cv.IsPivot && !cv.IsNestedObject {
		if col.Ref != "" {
			setRelationFields(&cv, normalizeEntityName(col.Ref), apiBase)
		} else if strings.HasSuffix(lowerJSON, "_id") {
			setRelationFields(&cv, normalizeEntityName(strings.TrimSuffix(lowerJSON, "_id")), apiBase)
		} else if lowerJSON != "id" && len(lowerJSON) > 2 && strings.HasSuffix(lowerJSON, "id") {
			rawEntity := strings.TrimRight(strings.TrimSuffix(lowerJSON, "id"), "_")
			if rawEntity != "" {
				setRelationFields(&cv, normalizeEntityName(rawEntity), apiBase)
			}
		}
	}

	// Type mapping (when component not yet specialized)
	if !cv.IsRelation {
		typeLower := strings.ToLower(col.Type)
		switch {
		case strings.Contains(typeLower, "int"), typeLower == "uint":
			cv.TSType = "number"
			cv.InputType = "number"
			cv.Align = "right"
		case strings.Contains(typeLower, "float"), strings.Contains(typeLower, "double"),
			strings.Contains(typeLower, "decimal"):
			cv.TSType = "number"
			cv.InputType = "number"
			cv.Align = "right"
		case typeLower == "bool", typeLower == "boolean":
			cv.TSType = "boolean"
			cv.Component = "q-toggle"
			cv.Align = "center"
		default:
			cv.TSType = "string"
			if cv.InputType == "text" && col.Constraints != nil {
				cv.InputType = mapFormatToInputType(col.Constraints.Format)
			}
		}
	}

	// Textarea detection by field name keywords (only for string q-input fields)
	if cv.Component == "q-input" && cv.TSType == "string" && !cv.IsNestedObject {
		textareaKW := []string{"description", "content", "body", "summary", "note", "comment", "bio", "text", "remark"}
		nameLower := strings.ToLower(col.Name)
		for _, kw := range textareaKW {
			if strings.Contains(nameLower, kw) {
				cv.IsTextarea = true
				cv.Sortable = false
				break
			}
		}
	}

	cv.QuasarRules = buildQuasarRules(cv, col)
	return cv
}

func setRelationFields(cv *ColumnView, target, apiBase string) {
	cv.IsRelation = true
	cv.Component = "q-select"
	cv.RelationEntity = toPascal(target)
	cv.RelationEntityLower = toCamel(target)
	cv.RelationEntityKebab = toKebab(target)
	cv.RelationAPIPath = apiBase + "/" + toKebab(toPlural(target))
}

func buildRelationView(rel *RelationNode, apiBase string, schema *ConsolidatedSchema) RelationView {
	target := normalizeEntityName(rel.TargetStruct)
	plural := toPlural(toPascal(target))
	rv := RelationView{
		FieldName:         rel.FieldName,
		TargetEntity:      toPascal(target),
		TargetLower:       toCamel(target),
		TargetKebab:       toKebab(target),
		TargetPlural:      plural,
		TargetPluralKebab: toKebab(plural),
		TargetAPIPath:     apiBase + "/" + toKebab(plural),
		TargetKey:         rel.TargetKey,
		SourceKey:         rel.SourceKey,
		IsCollection:      rel.IsCollection,
		Description:       rel.Description,
	}

	// Heuristic: Link Zod schemas from target entity's operations
	var targetMeta *TableMetadata
	normTarget := normalizeEntityName(target)
	// Find target in entity_list or entities map
	for _, m := range schema.EntityList {
		if m.NormalizedName == normTarget {
			targetMeta = m
			break
		}
	}
	if targetMeta == nil {
		targetMeta = schema.Entities[rel.TargetStruct]
	} else {
		for _, op := range targetMeta.Operations {
			if op.Method == "POST" && rv.TargetCreateSchema == "" && len(op.Tags) > 0 {
				if op.RequestSchema != "" {
					tag := op.Tags[0]
					pascalTag := toPascal(tag)
					kebabTag := toKebab(tag)
					rv.TargetCreateSchema = "Post" + pascalTag + "Body"
					if rv.ZodImportPath == "" {
						rv.ZodImportPath = "../../api/gen/zod/" + kebabTag + "/" + kebabTag
					}
				}
			}
			if (op.Method == "PUT" || op.Method == "PATCH") && rv.TargetUpdateSchema == "" && len(op.Tags) > 0 {
				if op.RequestSchema != "" {
					tag := op.Tags[0]
					pascalTag := toPascal(tag)
					kebabTag := toKebab(tag)
					rv.TargetUpdateSchema = strings.Title(op.Method) + pascalTag + "IdBody"
					if rv.ZodImportPath == "" {
						rv.ZodImportPath = "../../api/gen/zod/" + kebabTag + "/" + kebabTag
					}
				}
			}
		}
	}

	return rv
}

// ======================== Detection Helpers ========================

func detectPrimaryKey(cols []ColumnView) string {
	for _, c := range cols {
		if strings.ToLower(c.JSONName) == "id" {
			return c.JSONName
		}
	}
	for _, c := range cols {
		if strings.Contains(strings.ToLower(c.JSONName), "id") && c.TSType == "number" {
			return c.JSONName
		}
	}
	if len(cols) > 0 {
		return cols[0].JSONName
	}
	return "id"
}

func detectDisplayField(cols []ColumnView, pk string) string {
	candidates := []string{"name", "title", "label", "username", "email", "slug", "display_name", "displayname"}
	for _, c := range candidates {
		for _, cv := range cols {
			if strings.ToLower(cv.JSONName) == c {
				return cv.JSONName
			}
		}
	}
	for _, cv := range cols {
		if cv.TSType == "string" && cv.JSONName != pk && !cv.IsPrimaryKey {
			return cv.JSONName
		}
	}
	return pk
}

func mapFormatToInputType(format string) string {
	switch strings.ToLower(format) {
	case "email":
		return "email"
	case "date", "date-time":
		return "date"
	case "uri", "url":
		return "url"
	case "password":
		return "password"
	case "time":
		return "time"
	default:
		return "text"
	}
}

// ======================== Validation ========================

func buildQuasarRules(cv ColumnView, col ColumnInfo) string {
	var rules []string

	if cv.Required {
		rules = append(rules, fmt.Sprintf(
			"(val: any) => (val !== null && val !== undefined && val !== '') || '%s is required'",
			escapeJSString(cv.Label)))
	}

	if col.Constraints != nil {
		c := col.Constraints
		if c.MinLength != nil {
			rules = append(rules, fmt.Sprintf(
				"(val: any) => !val || String(val).length >= %d || '%s must be at least %d characters'",
				*c.MinLength, escapeJSString(cv.Label), *c.MinLength))
		}
		if c.MaxLength != nil {
			rules = append(rules, fmt.Sprintf(
				"(val: any) => !val || String(val).length <= %d || '%s must be at most %d characters'",
				*c.MaxLength, escapeJSString(cv.Label), *c.MaxLength))
		}
		if c.Minimum != nil {
			rules = append(rules, fmt.Sprintf(
				"(val: any) => val === '' || val === null || Number(val) >= %g || '%s must be >= %g'",
				*c.Minimum, escapeJSString(cv.Label), *c.Minimum))
		}
		if c.Maximum != nil {
			rules = append(rules, fmt.Sprintf(
				"(val: any) => val === '' || val === null || Number(val) <= %g || '%s must be <= %g'",
				*c.Maximum, escapeJSString(cv.Label), *c.Maximum))
		}
		if c.Pattern != "" {
			rules = append(rules, fmt.Sprintf(
				"(val: any) => !val || /%s/.test(String(val)) || '%s format is invalid'",
				c.Pattern, escapeJSString(cv.Label)))
		}
		if c.Format == "email" {
			rules = append(rules, fmt.Sprintf(
				"(val: any) => !val || /^[^\\s@]+@[^\\s@]+\\.[^\\s@]+$/.test(val) || '%s must be a valid email'",
				escapeJSString(cv.Label)))
		}
	}

	if len(rules) == 0 {
		return "[]"
	}
	return "[\n    " + strings.Join(rules, ",\n    ") + ",\n  ]"
}

func formatEnumOptions(enums []string) string {
	if len(enums) == 0 {
		return "[]"
	}
	parts := make([]string, len(enums))
	for i, e := range enums {
		parts[i] = fmt.Sprintf("{ label: '%s', value: '%s' }", escapeJSString(toHuman(e)), escapeJSString(e))
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// ======================== Rendering ========================

func renderToFile(templates *template.Template, name, outPath string, data any) error {
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return fmt.Errorf("mkdir for %s: %w", outPath, err)
	}
	f, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("create %s: %w", outPath, err)
	}
	defer f.Close()

	tpl := templates.Lookup(name)
	if tpl == nil {
		return fmt.Errorf("template %q not found", name)
	}
	if err := tpl.Execute(f, data); err != nil {
		return fmt.Errorf("execute %s: %w", name, err)
	}
	fmt.Printf("  📄 %s\n", outPath)
	return nil
}

// ======================== String Utilities ========================

func splitWords(s string) []string {
	var words []string
	var current []rune

	flush := func() {
		if len(current) > 0 {
			words = append(words, string(current))
			current = current[:0]
		}
	}

	runes := []rune(s)
	for i, r := range runes {
		if r == '_' || r == '-' || r == ' ' || r == '.' {
			flush()
			continue
		}
		if unicode.IsUpper(r) && i > 0 {
			prev := runes[i-1]
			if unicode.IsLower(prev) {
				flush()
			} else if unicode.IsUpper(prev) && i+1 < len(runes) && unicode.IsLower(runes[i+1]) {
				flush()
			}
		}
		current = append(current, r)
	}
	flush()
	return words
}

func toPascal(s string) string {
	words := splitWords(s)
	for i, w := range words {
		if w == "" {
			continue
		}
		r := []rune(strings.ToLower(w))
		r[0] = unicode.ToUpper(r[0])
		words[i] = string(r)
	}
	return strings.Join(words, "")
}

func toCamel(s string) string {
	p := toPascal(s)
	if p == "" {
		return p
	}
	r := []rune(p)
	r[0] = unicode.ToLower(r[0])
	return string(r)
}

func toKebab(s string) string {
	words := splitWords(s)
	for i := range words {
		words[i] = strings.ToLower(words[i])
	}
	return strings.Join(words, "-")
}

func toSnake(s string) string {
	words := splitWords(s)
	for i := range words {
		words[i] = strings.ToLower(words[i])
	}
	return strings.Join(words, "_")
}

func toHuman(s string) string {
	words := splitWords(s)
	for i, w := range words {
		if w == "" {
			continue
		}
		r := []rune(strings.ToLower(w))
		r[0] = unicode.ToUpper(r[0])
		words[i] = string(r)
	}
	return strings.Join(words, " ")
}

func toPlural(s string) string {
	if s == "" {
		return s
	}
	lower := strings.ToLower(s)
	for _, suf := range []string{"ies", "ses", "xes", "zes", "ches", "shes"} {
		if strings.HasSuffix(lower, suf) {
			return s
		}
	}
	switch {
	case strings.HasSuffix(lower, "s"), strings.HasSuffix(lower, "x"),
		strings.HasSuffix(lower, "z"), strings.HasSuffix(lower, "ch"),
		strings.HasSuffix(lower, "sh"):
		return s + "es"
	case strings.HasSuffix(lower, "y") && len(lower) > 1:
		beforeY := lower[len(lower)-2]
		if beforeY != 'a' && beforeY != 'e' && beforeY != 'i' && beforeY != 'o' && beforeY != 'u' {
			return s[:len(s)-1] + "ies"
		}
		return s + "s"
	default:
		return s + "s"
	}
}

func normalizeEntityName(name string) string {
	cleaned := name
	suffixes := []string{
		"Req", "Request", "Res", "Response", "Input", "Output",
		"Create", "Update", "Add", "Edit", "Delete",
		"Item", "Detail", "List", "Get", "Query", "Form",
		"Dto", "DTO",
	}
	prefixes := []string{"V1", "V2", "Api"}
	for _, pre := range prefixes {
		cleaned = strings.TrimPrefix(cleaned, pre)
	}
	for _, suf := range suffixes {
		cleaned = strings.TrimSuffix(cleaned, suf)
	}
	if cleaned == "" {
		return name
	}
	return cleaned
}

func escapeJSString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `'`, `\'`)
	return s
}
