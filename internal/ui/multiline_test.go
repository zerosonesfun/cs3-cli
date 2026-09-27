package ui

import "testing"

func TestLineEndsMultiline(t *testing.T) {
	if !lineEndsMultiline(".", ".") {
		t.Fatal("exact dot should end")
	}
	if !lineEndsMultiline("  .  ", ".") {
		t.Fatal("dot with surrounding space should end")
	}
	if !lineEndsMultiline(".\r", ".") {
		t.Fatal("dot with CR should end")
	}
	if lineEndsMultiline("..", ".") {
		t.Fatal("two dots should not end")
	}
	if lineEndsMultiline(".a", ".") {
		t.Fatal("dot plus text should not end")
	}
	if lineEndsMultiline("", ".") {
		t.Fatal("blank line should not end")
	}
}
