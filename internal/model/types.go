package model

import "time"

const (
	KindFrontmatter = "frontmatter"
	KindParagraph   = "paragraph"
	KindHeading     = "heading"
	KindCode        = "code"
	KindList        = "list"
	KindQuote       = "quote"
	KindTable       = "table"
	KindHr          = "hr"
	KindHtml        = "html"
	KindMath        = "math"
	KindFootnote    = "footnote_def"
	KindLinkDef     = "link_def"
)

// FrontMatter stores parsed YAML frontmatter values.
type FrontMatter map[string]interface{}

// Document is a markdown file parsed into tables-friendly objects.
type Document struct {
	Path            string
	RelPath         string
	Content         string
	Lines           []string
	Size            int64
	ModTime         time.Time
	FrontMatter     FrontMatter
	FrontMatterRaw  string
	FrontMatterFrom int
	FrontMatterTo   int
	Warnings        []string

	Headings  []Heading
	Sections  []Section
	Blocks    []Block
	Tasks     []Task
	Links     []Link
	Tags      []Tag
	Quotes    []Quote
	LinesRows []Line
}

// Heading is a markdown heading block.
type Heading struct {
	File       string
	Start      int
	End        int
	Level      int
	Title      string
	Text       string
	Slug       string
	Section    string
	Path       string
	Nested     bool
	SectionEnd int
	Parent     string
	Depth      int
	Raw        string
}

// Section is a logical section derived from top-level headings.
type Section struct {
	File       string
	Level      int
	Title      string
	Text       string
	Slug       string
	Path       string
	Start      int
	End        int
	Parent     string
	Depth      int
	Raw        string
	Body       string
	Own        string
	Children   int
	Headings   int
	Tasks      int
	OpenTasks  int
	CodeBlocks int
	Lines      int
}

// Block is one normalized markdown block.
type Block struct {
	File    string
	Kind    string
	Start   int
	End     int
	Line    int
	Section string
	Path    string
	Text    string
	Raw     string
	Lang    string
	Info    string
	Fence   string
	Nested  bool
	Level   int
}

// Task is an Obsidian task list item with metadata.
type Task struct {
	Block
	Text         string
	Status       string
	State        string
	Container    string
	Done         bool
	Due          *time.Time
	Scheduled    *time.Time
	StartDate    *time.Time
	DoneAt       *time.Time
	CancelledAt  *time.Time
	Created      *time.Time
	Recurrence   string
	ID           string
	DependsOn    []string
	Priority     int
	PriorityName string
	Fields       map[string]interface{}
	Depth        int
	Parent       int
	Root         int
	Tags         []string
	Section      string
	SectionPath  string
}

// Link is one parsed markdown hyperlink.
type Link struct {
	File     string
	Type     string
	Source   string
	Key      string
	Target   string
	Anchor   string
	BlockRef string
	Text     string
	Raw      string
	Line     int
	Section  string
	Path     string
	External bool
}

// Tag is one tag hit.
type Tag struct {
	File      string
	Tag       string
	Source    string
	Line      int
	Section   string
	Path      string
	BlockKind string
}

// Quote is an Obsidian/Markdown blockquote callout.
type Quote struct {
	File    string
	Start   int
	End     int
	Type    string
	Title   string
	Fold    string
	Body    string
	Text    string
	Path    string
	Section string
}

// Line is a single source line projection.
type Line struct {
	File    string
	N       int
	Text    string
	Kind    string
	Section string
	Path    string
}
