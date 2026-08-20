package web

import (
	"encoding/json"
	"html/template"
	"net/http"
	"os"
	"sort"
	"time"
)

// blogManifestEntry is one row of web/content/blog/manifest.json. Blog posts
// are plain files, not database rows — Adam is the sole author and edits
// happen via git, so there's no repository/service layer or authoring UI,
// matching how site designs are already read from disk at runtime.
type blogManifestEntry struct {
	Slug        string `json:"slug"`
	Title       string `json:"title"`
	Description string `json:"description"`
	PublishedAt string `json:"publishedAt"` // "2006-01-02"
	Draft       bool   `json:"draft"`
}

func loadBlogManifest() ([]blogManifestEntry, error) {
	data, err := os.ReadFile("web/content/blog/manifest.json")
	if err != nil {
		return nil, err
	}
	var entries []blogManifestEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

func formatBlogDate(s string) string {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return s
	}
	return t.Format("Jan 2, 2006")
}

// BlogIndex lists published posts, newest first.
func (h *Handler) BlogIndex(w http.ResponseWriter, r *http.Request) {
	entries, err := loadBlogManifest()
	if err != nil {
		h.render.RenderError(w, http.StatusInternalServerError)
		return
	}

	type post struct {
		Slug, Title, Description, PublishedAt string
		rawDate                               string
	}
	var posts []post
	for _, e := range entries {
		if e.Draft {
			continue
		}
		posts = append(posts, post{Slug: e.Slug, Title: e.Title, Description: e.Description, PublishedAt: formatBlogDate(e.PublishedAt), rawDate: e.PublishedAt})
	}
	sort.Slice(posts, func(i, j int) bool { return posts[i].rawDate > posts[j].rawDate })

	var featured *post
	rest := posts
	if len(posts) > 0 {
		featured = &posts[0]
		rest = posts[1:]
	}

	_, loggedIn := h.auth.CheckUser(w, r)
	h.render.Render(w, "blog_index", map[string]any{
		"Featured":        featured,
		"Posts":           rest,
		"LoggedIn":        loggedIn,
		"PageTitle":       "Blog | Launchly",
		"PageDescription": "Notes on building Launchly, and what actually matters for a local business website.",
	})
}

// BlogPost renders a single post by slug.
func (h *Handler) BlogPost(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")

	entries, err := loadBlogManifest()
	if err != nil {
		h.render.RenderError(w, http.StatusInternalServerError)
		return
	}

	var entry *blogManifestEntry
	for i := range entries {
		if entries[i].Slug == slug && !entries[i].Draft {
			entry = &entries[i]
			break
		}
	}
	if entry == nil {
		h.render.RenderError(w, http.StatusNotFound)
		return
	}

	body, err := os.ReadFile("web/content/blog/posts/" + entry.Slug + ".html")
	if err != nil {
		h.render.RenderError(w, http.StatusNotFound)
		return
	}

	_, loggedIn := h.auth.CheckUser(w, r)
	h.render.Render(w, "blog_post", map[string]any{
		"Post": map[string]any{
			"Title":       entry.Title,
			"PublishedAt": formatBlogDate(entry.PublishedAt),
			"BodyHTML":    template.HTML(body), // trusted, author-written content — not user input
		},
		"LoggedIn":        loggedIn,
		"PageTitle":       entry.Title + " | Launchly Blog",
		"PageDescription": entry.Description,
	})
}
