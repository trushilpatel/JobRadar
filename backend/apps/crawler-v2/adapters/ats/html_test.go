package ats

import "testing"

func TestStripHTML(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty", "", ""},
		{"plain text", "Hello World", "Hello World"},
		{"basic tags", "<p>Hello World</p>", "Hello World"},
		{"entity decode", "<p>Hello &amp; World</p>", "Hello & World"},
		{"nested tags", "<div><p>Hello</p><span> World</span></div>", "Hello World"},
		{"whitespace collapse", "<p>Hello  \n  World</p>", "Hello World"},
		{"only whitespace", "   ", ""},
		{"multiple entities", "&lt;br&gt; &amp; &quot;quote&quot;", "<br> & \"quote\""},
		{"self-closing tag", "Hello<br/>World", "Hello World"},
		{"ul li list", "<ul><li>Item 1</li><li>Item 2</li></ul>", "Item 1 Item 2"},
		{"style attr", `<p style="color:red">Red text</p>`, "Red text"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StripHTML(tt.input)
			if got != tt.want {
				t.Errorf("StripHTML(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
