package main

import (
	"reflect"
	"testing"
)

func TestSplitSentences(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{
			name: "two sentences",
			in:   "LoadPortfolio reads every file. It returns an error if validation fails.",
			want: []string{
				"LoadPortfolio reads every file.",
				"It returns an error if validation fails.",
			},
		},
		{
			// Matches pydocreflow.py's actual behavior: the abbreviation guard checks the run of
			// letters immediately before the trailing period of "e.g.", which is just "g" (the
			// internal period breaks the run), so it does not suppress this split. Verified against
			// the reference implementation directly; this is an accepted quirk, not a regression.
			name: "abbreviation guard does not reach across the internal period",
			in:   "Use small inputs, e.g. a single file, to keep tests fast.",
			want: []string{"Use small inputs, e.g.", "a single file, to keep tests fast."},
		},
		{
			name: "url does not split",
			in:   "See https://go.dev/s/generatedcode. for the convention.",
			want: []string{"See https://go.dev/s/generatedcode. for the convention."},
		},
		{
			name: "list marker guard suppresses split after the number",
			in:   "Step 1. Build the binary.",
			want: []string{"Step 1. Build the binary."},
		},
		{
			name: "no terminal punctuation",
			in:   "GetUser retrieves a user by ID from the database",
			want: []string{"GetUser retrieves a user by ID from the database"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := splitSentences(c.in)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("splitSentences(%q) = %#v, want %#v", c.in, got, c.want)
			}
		})
	}
}

func TestSplitClauses(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{
			name: "conjunction clause split",
			in:   "It merges the valid files into a single Portfolio, and it returns an error if any file fails validation.",
			want: []string{
				"It merges the valid files into a single Portfolio,",
				"and it returns an error if any file fails validation.",
			},
		},
		{
			name: "oxford comma list stays intact",
			in:   "It accepts strings, ints, and floats.",
			want: []string{"It accepts strings, ints, and floats."},
		},
		{
			name: "semicolon split",
			in:   "The cache is populated lazily; callers must not assume it is warm at startup.",
			want: []string{
				"The cache is populated lazily;",
				"callers must not assume it is warm at startup.",
			},
		},
		{
			name: "semicolon participle continuation does not split",
			in:   "The result is cached; serialized to JSON on first use.",
			want: []string{"The result is cached; serialized to JSON on first use."},
		},
		{
			name: "compound predicate does not split",
			in:   "It validates the request and writes a response.",
			want: []string{"It validates the request and writes a response."},
		},
		{
			name: "comma in parens does not split",
			in:   "It logs the result (success, failure, or timeout) and returns.",
			want: []string{"It logs the result (success, failure, or timeout) and returns."},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := splitClauses(c.in)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("splitClauses(%q) = %#v, want %#v", c.in, got, c.want)
			}
		})
	}
}

func TestReflowText(t *testing.T) {
	in := "LoadPortfolio reads every position file in dir and validates each one against the\nschema. It merges the valid files into a single Portfolio, and it returns an error\nif any file fails validation or two files declare the same position ID."
	want := []string{
		"LoadPortfolio reads every position file in dir and validates each one against the schema.",
		"It merges the valid files into a single Portfolio,",
		"and it returns an error if any file fails validation or two files declare the same position ID.",
	}
	got := reflowText(in)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("reflowText = %#v, want %#v", got, want)
	}
}
