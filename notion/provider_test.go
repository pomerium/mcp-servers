package notion

import (
	"testing"

	"github.com/jomei/notionapi"
)

func TestExtractRichText(t *testing.T) {
	richText := []notionapi.RichText{
		{
			Type:      "text",
			PlainText: "Hello ",
		},
		{
			Type:      "text",
			PlainText: "world!",
		},
	}

	result := extractRichText(richText)
	expected := "Hello world!"

	if result != expected {
		t.Errorf("Expected %q, got %q", expected, result)
	}
}

func TestIndentText(t *testing.T) {
	input := "Line 1\nLine 2\nLine 3"
	expected := "  Line 1\n  Line 2\n  Line 3"

	result := indentText(input)

	if result != expected {
		t.Errorf("Expected %q, got %q", expected, result)
	}
}
