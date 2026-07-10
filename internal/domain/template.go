package domain

// Template describes a built-in site design available in the builder.
// The catalog of templates is defined in code (internal/web/handlers), not
// stored in the database, since templates are compiled assets, not user data.
type Template struct {
	ID          string
	Name        string
	Description string
	Category    string // "general" or an industry name, e.g. "salon"
	Palettes    []Palette
}

// Palette is one selectable colour scheme for a template.
type Palette struct {
	ID   string
	Name string
	CSS  string // inline CSS custom-property overrides
}
