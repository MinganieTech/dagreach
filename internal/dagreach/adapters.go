package dagreach

// Profiles: what a producer's export means, so you do not have to say it.
//
// DOT and JSON carry structure. They do not carry which way an edge points,
// what an identifier is, or what counts as a group - and getting those wrong
// produces confident, wrong answers. A profile is the small piece of knowledge
// that turns one producer's export into a graph dagreach can reason about: the
// format to read, the edge semantics that producer uses, and the identifier and
// attribute conventions worth normalising.
//
// Two rules keep a profile trustworthy rather than magic: Load returns the graph
// in the producer's own orientation (the single reversal happens later, in
// Orient), and anything a profile decided is visible in the report.

import (
	"fmt"
	"regexp"
	"strings"
)

// Profile holds one producer's conventions.
type Profile struct {
	Name          string
	Summary       string
	ProducedBy    string
	EdgeSemantics string
	Load          func(text, source string) (*Graph, error)
	Detect        func(text string) bool
}

// ProfileOrder is the order profiles are listed and detection is attempted in.
var ProfileOrder = []string{"terraform", "dbt", "cyclonedx", "generic"}

var profiles = map[string]*Profile{
	"terraform": {
		Name:          "terraform",
		Summary:       "strips the [root] decoration, groups by resource kind",
		ProducedBy:    "terraform graph",
		EdgeSemantics: "depends-on",
		Load:          loadTerraform,
		Detect:        detectTerraform,
	},
	"dbt": {
		Name:       "dbt",
		Summary:    "reads a manifest offline, groups by resource type, keeps tags and materialisation",
		ProducedBy: "dbt (target/manifest.json)",

		EdgeSemantics: "feeds",
		Load:          loadDBT,
		Detect:        detectDBT,
	},
	"cyclonedx": {
		Name:          "cyclonedx",
		Summary:       "reads an SBOM, groups by component type, keeps versions and licences",
		ProducedBy:    "CycloneDX (syft, cdxgen, trivy, ...)",
		EdgeSemantics: "depends-on",
		Load:          loadCycloneDX,
		Detect:        detectCycloneDX,
	},
	"generic": {
		Name:          "generic",
		Summary:       "DOT or JSON Graph Format, no normalisation, semantics up to you",
		ProducedBy:    "anything",
		EdgeSemantics: DefaultSemantics,
		Load:          nil, // handled by the reader, which needs the file name for detection
		Detect:        func(string) bool { return false },
	},
}

// GetProfile returns a profile by name.
func GetProfile(name string) (*Profile, bool) {
	profile, ok := profiles[name]
	return profile, ok
}

// Profiles returns every profile, in listing order.
func Profiles() []*Profile {
	listed := make([]*Profile, 0, len(ProfileOrder))
	for _, name := range ProfileOrder {
		listed = append(listed, profiles[name])
	}
	return listed
}

// DetectProfile recognises a producer from the content, or returns nil rather
// than guess. `generic` never matches here: it is what runs when nothing was
// recognised, and a detection that always succeeds would tell the reader nothing.
func DetectProfile(text string) *Profile {
	for _, name := range ProfileOrder {
		profile := profiles[name]
		if profile.Name == "generic" {
			continue
		}
		if profile.Detect(text) {
			return profile
		}
	}
	return nil
}

// -- terraform -------------------------------------------------------------

var (
	terraformPrefix = regexp.MustCompile(`^\[root\]\s+`)
	terraformSuffix = regexp.MustCompile(`\s+\((expand|close|destroy)\)$`)
)

// NormaliseTerraform drops the renderer's decoration:
// `[root] aws_vpc.main (expand)` -> `aws_vpc.main`.
func NormaliseTerraform(id string) string {
	return strings.TrimSpace(terraformSuffix.ReplaceAllString(terraformPrefix.ReplaceAllString(id, ""), ""))
}

// TerraformKind is the resource kind, used as the group: `aws_vpc.main` -> `aws_vpc`.
func TerraformKind(id string) string {
	if strings.HasPrefix(id, "provider[") {
		return "provider"
	}
	head, _, _ := strings.Cut(id, ".")
	if head == "" {
		return "unknown"
	}
	return head
}

func loadTerraform(text, source string) (*Graph, error) {
	raw, err := ParseDOT(text, source)
	if err != nil {
		return nil, err
	}

	mapping := map[string]string{}
	for _, id := range raw.Nodes() {
		mapping[id] = NormaliseTerraform(id)
	}
	if collisions := terraformCollisions(raw, mapping); len(collisions) > 0 {
		raw.Warn(fmt.Sprintf(
			"%d identifier(s) would collide once the [root] decoration is stripped, "+
				"so those nodes keep their full terraform identifier", len(collisions)))
		for _, id := range raw.Nodes() {
			if collisions[mapping[id]] {
				mapping[id] = id
			}
		}
	}

	graph := NewGraph(source)
	graph.Name = raw.Name
	graph.Format = "dot"
	for key, value := range raw.Attrs {
		graph.Attrs[key] = value
	}
	graph.Warnings = append(graph.Warnings, raw.Warnings...)

	for _, id := range raw.Nodes() {
		attrs := copyAttrs(raw.Node(id).Attrs)
		attrs[GroupKey] = TerraformKind(mapping[id])
		if mapping[id] != id {
			attrs["terraform_id"] = id
		}
		graph.AddNode(mapping[id], attrs, true)
	}
	for _, edge := range raw.Edges {
		graph.AddEdge(mapping[edge.Source], mapping[edge.Target], edge.Attrs)
	}
	return graph, nil
}

func terraformCollisions(g *Graph, mapping map[string]string) map[string]bool {
	seen := map[string]string{}
	collisions := map[string]bool{}
	for _, id := range g.Nodes() {
		stripped := mapping[id]
		if previous, present := seen[stripped]; present && previous != id {
			collisions[stripped] = true
		}
		seen[stripped] = id
	}
	return collisions
}

func detectTerraform(text string) bool {
	head := text
	if len(head) > 4000 {
		head = head[:4000]
	}
	return strings.Contains(head, "[root] ") &&
		(strings.Contains(head, `compound = "true"`) || strings.Contains(head, `newrank = "true"`))
}

// -- dbt -------------------------------------------------------------------

var dbtSections = []string{"nodes", "sources", "exposures", "metrics", "semantic_models"}

func loadDBT(text, source string) (*Graph, error) {
	document, err := DecodeOrderedJSON(text)
	if err != nil {
		line, column := jsonErrorPosition(text, err)
		return nil, &ParseError{Message: "invalid JSON: " + err.Error(),
			Source: source, Line: line, Column: column}
	}
	manifest, ok := document.(*Object)
	if !ok {
		return nil, &ParseError{Message: "a dbt manifest must be a JSON object", Source: source}
	}

	graph := NewGraph(source)
	graph.Format = "dbt"
	if metadata, ok := manifest.Value("metadata").(*Object); ok {
		if name, ok := asText(metadata.Value("project_name")); ok {
			graph.Name = name
		}
		for _, key := range []string{"dbt_version", "dbt_schema_version", "adapter_type"} {
			if value, ok := metadata.Value(key).(string); ok {
				graph.Attrs[key] = value
			}
		}
	}

	declared := map[string]*Object{}
	for _, section := range dbtSections {
		entries, ok := manifest.Value(section).(*Object)
		if !ok {
			continue
		}
		for _, uniqueID := range entries.Keys() {
			entry, ok := entries.Value(uniqueID).(*Object)
			if !ok {
				continue
			}
			declared[uniqueID] = entry
			graph.AddNode(uniqueID, dbtAttrs(uniqueID, entry), true)
		}
	}

	childMap, hasChildMap := manifest.Value("child_map").(*Object)
	if hasChildMap && childMap.Len() > 0 {
		for _, parent := range childMap.Keys() {
			children, _ := childMap.Value(parent).([]any)
			for _, child := range children {
				graph.AddEdge(parent, asString(child), nil)
			}
		}
	} else {
		graph.Warn("no 'child_map' in this manifest; edges were read from depends_on.nodes")
		for _, uniqueID := range graph.Nodes() {
			entry := declared[uniqueID]
			dependsOn, _ := entry.Value("depends_on").(*Object)
			parents, _ := dependsOn.Value("nodes").([]any)
			for _, parent := range parents {
				graph.AddEdge(asString(parent), uniqueID, nil)
			}
		}
	}

	undeclared := 0
	for _, id := range graph.Nodes() {
		if _, ok := declared[id]; !ok {
			undeclared++
		}
	}
	if undeclared > 0 {
		graph.Warn(fmt.Sprintf(
			"%d node(s) appear in the dependency maps but in no section of the manifest "+
				"(macros and tests of other packages usually explain this)", undeclared))
	}
	return graph, nil
}

func dbtAttrs(uniqueID string, entry *Object) map[string]string {
	attrs := map[string]string{}
	resourceType, ok := asText(entry.Value("resource_type"))
	if !ok || resourceType == "" {
		resourceType, _, _ = strings.Cut(uniqueID, ".")
	}
	attrs[GroupKey] = resourceType
	for _, key := range []string{"name", "package_name", "schema", "database", "path"} {
		if value, ok := entry.Value(key).(string); ok {
			attrs[key] = value
		}
	}
	if config, ok := entry.Value("config").(*Object); ok {
		if materialized, ok := config.Value("materialized").(string); ok {
			attrs["materialized"] = materialized
		}
		if tags, ok := config.Value("tags").([]any); ok && len(tags) > 0 {
			rendered := make([]string, 0, len(tags))
			for _, tag := range tags {
				rendered = append(rendered, asString(tag))
			}
			attrs["tags"] = strings.Join(rendered, ",")
		}
	}
	return attrs
}

func detectDBT(text string) bool {
	head := text
	if len(head) > 4000 {
		head = head[:4000]
	}
	return strings.Contains(head, `"dbt_schema_version"`) ||
		(strings.Contains(head, `"dbt_version"`) && strings.Contains(head, `"nodes"`))
}

// -- cyclonedx -------------------------------------------------------------

func loadCycloneDX(text, source string) (*Graph, error) {
	document, err := DecodeOrderedJSON(text)
	if err != nil {
		line, column := jsonErrorPosition(text, err)
		return nil, &ParseError{Message: "invalid JSON: " + err.Error(),
			Source: source, Line: line, Column: column}
	}
	bom, ok := document.(*Object)
	if !ok {
		return nil, &ParseError{Message: "a CycloneDX document must be a JSON object", Source: source}
	}

	graph := NewGraph(source)
	graph.Format = "cyclonedx"
	for _, key := range []string{"bomFormat", "specVersion", "serialNumber", "version"} {
		if value := bom.Value(key); value != nil {
			graph.Attrs[key] = asString(value)
		}
	}

	if metadata, ok := bom.Value("metadata").(*Object); ok {
		if root, ok := metadata.Value("component").(*Object); ok {
			if name, ok := asText(root.Value("name")); ok {
				graph.Name = name
			}
			graph.AddNode(componentReference(root), componentAttrs(root, true), true)
		}
	}
	if components, ok := bom.Value("components").([]any); ok {
		for _, entry := range components {
			if component, ok := entry.(*Object); ok {
				graph.AddNode(componentReference(component), componentAttrs(component, false), true)
			}
		}
	}

	dependencies, ok := bom.Value("dependencies").([]any)
	if !ok {
		graph.Warn("no 'dependencies' array: the SBOM lists components but no relationships")
		return graph, nil
	}

	known := map[string]bool{}
	for _, id := range graph.Nodes() {
		known[id] = true
	}
	for _, entry := range dependencies {
		relation, ok := entry.(*Object)
		if !ok {
			continue
		}
		ref, ok := relation.Value("ref").(string)
		if !ok {
			continue
		}
		targets, _ := relation.Value("dependsOn").([]any)
		for _, target := range targets {
			graph.AddEdge(ref, asString(target), nil)
		}
	}

	undeclared := 0
	for _, id := range graph.Nodes() {
		if !known[id] {
			undeclared++
		}
	}
	if undeclared > 0 {
		graph.Warn(fmt.Sprintf(
			"%d reference(s) appear in 'dependencies' but in no component entry", undeclared))
	}
	return graph, nil
}

func componentReference(component *Object) string {
	for _, key := range []string{"bom-ref", "purl"} {
		if value, ok := component.Value(key).(string); ok && value != "" {
			return value
		}
	}
	name, ok := component.Value("name").(string)
	if !ok || name == "" {
		name = "unnamed"
	}
	if version, ok := asText(component.Value("version")); ok {
		return name + "@" + version
	}
	return name
}

func componentAttrs(component *Object, isRoot bool) map[string]string {
	group := "root"
	if !isRoot {
		group = "library"
		if kind, ok := component.Value("type").(string); ok && kind != "" {
			group = kind
		}
	}
	attrs := map[string]string{GroupKey: group}
	for _, key := range []string{"name", "version", "purl", "publisher"} {
		if value, ok := component.Value(key).(string); ok {
			attrs[key] = value
		}
	}
	if licences := componentLicences(component.Value("licenses")); licences != "" {
		attrs["licenses"] = licences
	}
	return attrs
}

func componentLicences(licenses any) string {
	listed, ok := licenses.([]any)
	if !ok {
		return ""
	}
	names := []string{}
	for _, entry := range listed {
		object, ok := entry.(*Object)
		if !ok {
			continue
		}
		if licence, ok := object.Value("license").(*Object); ok {
			if value, ok := licence.Value("id").(string); ok {
				names = append(names, value)
			} else if value, ok := licence.Value("name").(string); ok {
				names = append(names, value)
			}
		}
		if expression, ok := object.Value("expression").(string); ok {
			names = append(names, expression)
		}
	}
	return strings.Join(names, ",")
}

func detectCycloneDX(text string) bool {
	head := text
	if len(head) > 2000 {
		head = head[:2000]
	}
	return strings.Contains(head, `"bomFormat"`) && strings.Contains(head, "CycloneDX")
}
