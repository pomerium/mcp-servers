package notion

import (
	"context"
	_ "embed"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/jomei/notionapi"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/pomerium/mcp-servers/ctxutil"
	"github.com/pomerium/mcp-servers/drutil"
	"github.com/pomerium/mcp-servers/httputil"
)

func New(context.Context) drutil.Provider {
	return &notion{
		http: httputil.NewDebugHTTPClient(func(s string) { fmt.Println(s) }),
	}
}

func NewServer(ctx context.Context, _ map[string]string) (*mcp.Server, error) {
	provider := New(ctx)
	mcpServer := drutil.BuildMCPServer("Notion", provider)
	return mcpServer, nil
}

//go:embed search.txt
var searchDescription string

type notion struct {
	http *http.Client
}

func (n *notion) GetSearchSyntax() string {
	return searchDescription
}

func (n *notion) getClient(ctx context.Context) (*notionapi.Client, error) {
	token, err := ctxutil.AuthorizationTokenFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("get token from context: %w", err)
	}
	return notionapi.NewClient(
		notionapi.Token(token),
		notionapi.WithHTTPClient(n.http),
	), nil
}

func (n *notion) Search(ctx context.Context, query string) ([]drutil.Document, error) {
	client, err := n.getClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("get notion client: %w", err)
	}
	if query == "" {
		return nil, fmt.Errorf("query cannot be empty")
	}

	resp, err := client.Search.Do(ctx, &notionapi.SearchRequest{
		Query: query,
		Filter: notionapi.SearchFilter{
			Value:    "page",
			Property: "object",
		},
	})
	if err != nil {
		return nil, fmt.Errorf("search api: %w", err)
	}

	var documents []drutil.Document
	for _, res := range resp.Results {
		switch res.GetObject() {
		case notionapi.ObjectTypePage:
			documents = append(documents, pageToDocument(res.(*notionapi.Page)))
		default:
			slog.Info("ignoring unsupported object type", "type", res.GetObject())
			continue
		}
	}
	return documents, nil
}

func (n *notion) Fetch(ctx context.Context, id string) (*drutil.Document, error) {
	client, err := n.getClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("get notion client: %w", err)
	}

	// Fetch the page
	page, err := client.Page.Get(ctx, notionapi.PageID(id))
	if err != nil {
		return nil, fmt.Errorf("get page: %w", err)
	}

	// Get the title
	title := getTitleText(page)

	// Fetch all content blocks recursively
	content, err := n.fetchPageContent(ctx, client, notionapi.BlockID(id))
	if err != nil {
		return nil, fmt.Errorf("fetch page content: %w", err)
	}

	return &drutil.Document{
		ID:    id,
		Title: title,
		Text:  content,
		URL:   &page.URL,
	}, nil
}

func pageToDocument(page *notionapi.Page) drutil.Document {
	txt := getTitleText(page)
	return drutil.Document{
		ID:    page.ID.String(),
		Title: txt,
		Text:  txt,
		URL:   &page.URL,
	}
}

func getTitleText(page *notionapi.Page) string {
	title, ok := page.Properties[string(notionapi.PropertyTypeTitle)].(*notionapi.TitleProperty)
	if !ok {
		slog.Warn("page does not have a title property", "page_id", page.ID)
		return ""
	}

	if len(title.Title) == 0 {
		return ""
	}
	var b strings.Builder
	for i, t := range title.Title {
		if i > 0 {
			b.WriteString(" ")
		}
		b.WriteString(t.PlainText)
	}
	return b.String()
}

// fetchPageContent recursively fetches all blocks and extracts text content
func (n *notion) fetchPageContent(ctx context.Context, client *notionapi.Client, blockID notionapi.BlockID) (string, error) {
	var content strings.Builder

	// Get all children blocks with pagination
	cursor := ""
	hasMore := true

	for hasMore {
		pagination := &notionapi.Pagination{}
		if cursor != "" {
			pagination.StartCursor = notionapi.Cursor(cursor)
		}

		response, err := client.Block.GetChildren(ctx, blockID, pagination)
		if err != nil {
			return "", fmt.Errorf("get block children: %w", err)
		}

		// Process each block
		for _, block := range response.Results {
			blockText := n.extractTextFromBlock(ctx, client, block)
			if blockText != "" {
				if content.Len() > 0 {
					content.WriteString("\n")
				}
				content.WriteString(blockText)
			}
		}

		hasMore = response.HasMore
		cursor = response.NextCursor
	}

	return content.String(), nil
}

// extractTextFromBlock extracts text content from a single block and its children
func (n *notion) extractTextFromBlock(ctx context.Context, client *notionapi.Client, block notionapi.Block) string {
	var text strings.Builder

	switch b := block.(type) {
	case *notionapi.ParagraphBlock:
		text.WriteString(extractRichText(b.Paragraph.RichText))
		if len(b.Paragraph.Children) > 0 {
			childText := n.extractTextFromChildren(ctx, client, b.Paragraph.Children)
			if childText != "" {
				text.WriteString("\n" + childText)
			}
		}

	case *notionapi.Heading1Block:
		text.WriteString("# " + extractRichText(b.Heading1.RichText))

	case *notionapi.Heading2Block:
		text.WriteString("## " + extractRichText(b.Heading2.RichText))

	case *notionapi.Heading3Block:
		text.WriteString("### " + extractRichText(b.Heading3.RichText))

	case *notionapi.BulletedListItemBlock:
		text.WriteString("• " + extractRichText(b.BulletedListItem.RichText))
		if len(b.BulletedListItem.Children) > 0 {
			childText := n.extractTextFromChildren(ctx, client, b.BulletedListItem.Children)
			if childText != "" {
				text.WriteString("\n" + indentText(childText))
			}
		}

	case *notionapi.NumberedListItemBlock:
		text.WriteString("1. " + extractRichText(b.NumberedListItem.RichText))
		if len(b.NumberedListItem.Children) > 0 {
			childText := n.extractTextFromChildren(ctx, client, b.NumberedListItem.Children)
			if childText != "" {
				text.WriteString("\n" + indentText(childText))
			}
		}

	case *notionapi.ToDoBlock:
		checkbox := "☐"
		if b.ToDo.Checked {
			checkbox = "☑"
		}
		text.WriteString(checkbox + " " + extractRichText(b.ToDo.RichText))
		if len(b.ToDo.Children) > 0 {
			childText := n.extractTextFromChildren(ctx, client, b.ToDo.Children)
			if childText != "" {
				text.WriteString("\n" + indentText(childText))
			}
		}

	case *notionapi.ToggleBlock:
		text.WriteString("▶ " + extractRichText(b.Toggle.RichText))
		if len(b.Toggle.Children) > 0 {
			childText := n.extractTextFromChildren(ctx, client, b.Toggle.Children)
			if childText != "" {
				text.WriteString("\n" + indentText(childText))
			}
		}

	case *notionapi.CalloutBlock:
		text.WriteString("💡 " + extractRichText(b.Callout.RichText))
		if len(b.Callout.Children) > 0 {
			childText := n.extractTextFromChildren(ctx, client, b.Callout.Children)
			if childText != "" {
				text.WriteString("\n" + indentText(childText))
			}
		}

	case *notionapi.QuoteBlock:
		text.WriteString("> " + extractRichText(b.Quote.RichText))
		if len(b.Quote.Children) > 0 {
			childText := n.extractTextFromChildren(ctx, client, b.Quote.Children)
			if childText != "" {
				text.WriteString("\n" + indentText(childText))
			}
		}

	case *notionapi.CodeBlock:
		language := b.Code.Language
		text.WriteString("```" + language + "\n" + extractRichText(b.Code.RichText) + "\n```")

	case *notionapi.DividerBlock:
		text.WriteString("---")

	case *notionapi.TableBlock:
		// For tables, we need to fetch table rows as children
		if hasChildren := block.GetHasChildren(); hasChildren {
			tableContent, err := n.fetchPageContent(ctx, client, block.GetID())
			if err == nil && tableContent != "" {
				text.WriteString(tableContent)
			}
		}

	case *notionapi.TableRowBlock:
		row := []string{}
		for _, cell := range b.TableRow.Cells {
			row = append(row, extractRichText(cell))
		}
		text.WriteString("| " + strings.Join(row, " | ") + " |")

	case *notionapi.ChildPageBlock:
		text.WriteString("📄 " + b.ChildPage.Title)

	case *notionapi.BookmarkBlock:
		text.WriteString("🔗 " + b.Bookmark.URL)
		if len(b.Bookmark.Caption) > 0 {
			text.WriteString(" - " + extractRichText(b.Bookmark.Caption))
		}

	case *notionapi.EmbedBlock:
		text.WriteString("🔗 " + b.Embed.URL)

	case *notionapi.ImageBlock:
		imageText := "🖼 Image"
		if b.Image.Type == notionapi.FileTypeExternal && b.Image.External != nil {
			imageText += ": " + b.Image.External.URL
		}
		if len(b.Image.Caption) > 0 {
			imageText += " - " + extractRichText(b.Image.Caption)
		}
		text.WriteString(imageText)

	case *notionapi.VideoBlock:
		videoText := "🎥 Video"
		if b.Video.Type == notionapi.FileTypeExternal && b.Video.External != nil {
			videoText += ": " + b.Video.External.URL
		}
		if len(b.Video.Caption) > 0 {
			videoText += " - " + extractRichText(b.Video.Caption)
		}
		text.WriteString(videoText)

	case *notionapi.FileBlock:
		fileText := "📎 File"
		if len(b.File.Caption) > 0 {
			fileText += " - " + extractRichText(b.File.Caption)
		}
		text.WriteString(fileText)

	default:
		// For blocks with children but no specific text extraction
		if hasChildren := block.GetHasChildren(); hasChildren {
			childContent, err := n.fetchPageContent(ctx, client, block.GetID())
			if err == nil && childContent != "" {
				text.WriteString(childContent)
			}
		}
	}

	return text.String()
}

// extractTextFromChildren processes child blocks
func (n *notion) extractTextFromChildren(ctx context.Context, client *notionapi.Client, children notionapi.Blocks) string {
	var content strings.Builder
	for _, child := range children {
		childText := n.extractTextFromBlock(ctx, client, child)
		if childText != "" {
			if content.Len() > 0 {
				content.WriteString("\n")
			}
			content.WriteString(childText)
		}
	}
	return content.String()
}

// extractRichText extracts plain text from rich text array
func extractRichText(richText []notionapi.RichText) string {
	var text strings.Builder
	for _, rt := range richText {
		text.WriteString(rt.PlainText)
	}
	return text.String()
}

// indentText adds indentation to text for nested content
func indentText(text string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if line != "" {
			lines[i] = "  " + line
		}
	}
	return strings.Join(lines, "\n")
}
