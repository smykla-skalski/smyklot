package filemerge

import (
	"fmt"
	"strings"
)

const (
	// deepestHeading is the deepest ATX heading Markdown has.
	deepestHeading = 6

	// shortestFence is the shortest run that opens a code fence.
	shortestFence = 3

	// widestIndent is the most leading spaces a heading or a fence may carry.
	// Four makes it an indented code block instead, which is the difference
	// between a fence and a picture of one.
	widestIndent = 3

	// mostBlankLines is how many blank lines in a row a merged document keeps.
)

// locate answers which of a document's headings a section addresses, as an
// index into them.
//
// A heading that appears more than once is refused unless the section says
// which occurrence it means. The engine this replaces took the first and said
// nothing, so a section operation aimed at the second "## Usage" in a document
// silently rewrote the first - and a heading match that ignored the level made
// "## Usage" and "### Usage" the same heading.
func locate(written []heading, section Section) (int, error) {
	level, title, ok := parseHeading(section.Heading)
	if !ok {
		return 0, fmt.Errorf(
			"%w: %q is not a heading; write it with its # marks",
			ErrInvalidSpec, section.Heading)
	}

	var matches []int

	for index, candidate := range written {
		if candidate.level == level && strings.EqualFold(candidate.title, title) {
			matches = append(matches, index)
		}
	}

	switch {
	case len(matches) == 0:
		return 0, fmt.Errorf("%w: no heading %q", ErrNothingAddressed, section.Heading)

	case section.Occurrence == 0 && len(matches) > 1:
		return 0, fmt.Errorf(
			"%w: %q appears %d times and nothing says which one is meant",
			ErrNothingAddressed, section.Heading, len(matches))

	case section.Occurrence == 0:
		return matches[0], nil

	case section.Occurrence > len(matches):
		return 0, fmt.Errorf(
			"%w: %q appears %d times, so there is no occurrence %d",
			ErrNothingAddressed, section.Heading, len(matches), section.Occurrence)

	default:
		return matches[section.Occurrence-1], nil
	}
}

// heading is a heading and where it was written.
type heading struct {
	level int
	title string
	line  int

	// span is how many lines the heading itself occupies: one for `## Setup`,
	// and the paragraph plus its underline for the form written as
	//
	//	Setup
	//	-----
	//
	// It is what tells a section's heading from a section's body, and the two
	// forms differ only in that.
	span int
}

// headings reads every heading in a document, skipping fenced code.
//
// Both forms CommonMark has. Reading only the `#` sort left a document's
// underlined headings invisible, so a section ran to the end of the file and
// replacing one deleted every underlined section below it - which is the
// silent destruction this whole engine was rewritten to stop.
func headings(lines []string) []heading {
	var (
		found  []heading
		fence  byte
		length int
		open   bool

		// Where the run of ordinary lines above this one started, or -1. A
		// heading written with an underline is that run, and the line under it.
		paragraph = -1
	)

	for index, text := range lines {
		character, run, rest, isFence := fenceAt(text)

		if open {
			if isFence && character == fence && run >= length && strings.TrimSpace(rest) == "" {
				open = false
			}

			continue
		}

		if isFence && opensFence(character, rest) {
			open, fence, length, paragraph = true, character, run, -1

			continue
		}

		if level, title, ok := parseHeading(text); ok {
			found = append(found, heading{level: level, title: title, line: index, span: 1})
			paragraph = -1

			continue
		}

		// Only under a paragraph. The same line after a blank one is a
		// thematic break, which is not a heading and must not bound a section.
		if level, ok := underline(text); ok && paragraph >= 0 {
			found = append(found, heading{
				level: level,
				title: strings.TrimSpace(strings.Join(lines[paragraph:index], " ")),
				line:  paragraph,
				span:  index - paragraph + 1,
			})
			paragraph = -1

			continue
		}

		if strings.TrimSpace(text) == "" {
			paragraph = -1

			continue
		}

		if paragraph < 0 {
			paragraph = index
		}
	}

	return found
}

// afterIndent is a line past the leading spaces a heading or a fence may carry,
// and whether it carries few enough of them to be one.
//
// Four spaces makes it an indented code block, and a code block is not a
// heading or a fence however much it looks like one. That is one CommonMark
// rule, and the three readers below it were each enforcing their own copy - two
// of which had already stopped being the same copy.
func afterIndent(text string) (rest string, ok bool) {
	indent := 0
	for indent < len(text) && text[indent] == ' ' {
		indent++
	}

	if indent > widestIndent || indent == len(text) {
		return "", false
	}

	return text[indent:], true
}

// underline reads a line as the underline of a heading written the second way.
func underline(text string) (level int, ok bool) {
	rest, indented := afterIndent(text)
	if !indented {
		return 0, false
	}

	character := rest[0]
	if character != '=' && character != '-' {
		return 0, false
	}

	// A run of the one character, and then nothing but the whitespace a line
	// may end with.
	run := strings.TrimRight(rest, " \t")
	if strings.Trim(run, string(character)) != "" {
		return 0, false
	}

	return levelOf(character), true
}

func levelOf(character byte) int {
	if character == '=' {
		return 1
	}

	return 2
}

// opensFence tells a fence that starts a block from a line that only looks like
// one. A backtick fence's info string cannot itself hold a backtick, which is
// what keeps a paragraph's inline code from opening a block that swallows the
// rest of the document.
func opensFence(character byte, rest string) bool {
	return character != '`' || !strings.Contains(rest, "`")
}

// fenceAt reads a line as a code fence: the character it is made of, how long
// the run is, and what follows it.
//
// The character and the length both matter, and the engine this replaces
// checked neither. It toggled a single flag on any line starting with three
// backticks or three tildes, so a backtick fence inside a tilde block closed it
// and inverted the rest of the document - and every heading after that point
// stopped existing.
func fenceAt(text string) (character byte, run int, rest string, ok bool) {
	line, indented := afterIndent(text)
	if !indented {
		return 0, 0, "", false
	}

	character = line[0]
	if character != '`' && character != '~' {
		return 0, 0, "", false
	}

	for run < len(line) && line[run] == character {
		run++
	}

	if run < shortestFence {
		return 0, 0, "", false
	}

	return character, run, line[run:], true
}

// parseHeading reads a line as an ATX heading.
func parseHeading(text string) (level int, title string, ok bool) {
	rest, indented := afterIndent(text)
	if !indented {
		return 0, "", false
	}

	for level < len(rest) && rest[level] == '#' {
		level++
	}

	if level == 0 || level > deepestHeading {
		return 0, "", false
	}

	after := rest[level:]
	if after != "" && after[0] != ' ' && after[0] != '\t' {
		return 0, "", false
	}

	// A closing run of # marks is decoration rather than part of the title.
	title = strings.TrimSpace(after)
	title = strings.TrimSpace(strings.TrimRight(title, "#"))

	return level, title, true
}

// sectionEnd is the line a section stops before: the next heading at its own
// level or above, or the end of the document.
func sectionEnd(found []heading, index, lines int) int {
	level := found[index].level

	for next := index + 1; next < len(found); next++ {
		if found[next].level <= level {
			return found[next].line
		}
	}

	return lines
}
