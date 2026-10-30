package files

import (
	"strings"
	"unicode"
)

// A deliberately small expression grammar, not a calculation engine. The
// lexical allowlist also rejects external links, names and impure functions.
func authorFormula(value string, sheets []string) bool {
	if !authorFormulaTokens(value, sheets) {
		return false
	}
	p := authorFormulaParser{s: value[1:]}
	return p.expression(0, 0) && p.peek() == ""
}

type authorFormulaParser struct {
	s     string
	pos   int
	token string
	end   int
}

func (p *authorFormulaParser) peek() string {
	if p.token != "" {
		return p.token
	}
	i := p.pos
	for i < len(p.s) && unicode.IsSpace(rune(p.s[i])) {
		i++
	}
	if i == len(p.s) {
		p.pos = i
		return ""
	}
	start := i
	c := p.s[i]
	i++
	if c == '\'' || c == '"' {
		for i < len(p.s) {
			if p.s[i] == c {
				i++
				if i < len(p.s) && p.s[i] == c {
					i++
					continue
				}
				break
			}
			i++
		}
	} else if c >= '0' && c <= '9' || c == '.' {
		for i < len(p.s) && (p.s[i] >= '0' && p.s[i] <= '9' || p.s[i] == '.') {
			i++
		}
		if i < len(p.s) && (p.s[i] == 'e' || p.s[i] == 'E') {
			i++
			if i < len(p.s) && (p.s[i] == '+' || p.s[i] == '-') {
				i++
			}
			for i < len(p.s) && p.s[i] >= '0' && p.s[i] <= '9' {
				i++
			}
		}
	} else if c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c == '$' || c == '_' {
		for i < len(p.s) && (p.s[i] >= 'A' && p.s[i] <= 'Z' || p.s[i] >= 'a' && p.s[i] <= 'z' || p.s[i] >= '0' && p.s[i] <= '9' || p.s[i] == '$' || p.s[i] == '_') {
			i++
		}
	} else if i < len(p.s) && (c == '<' && (p.s[i] == '=' || p.s[i] == '>') || c == '>' && p.s[i] == '=') {
		i++
	}
	p.token, p.end = p.s[start:i], i
	return p.token
}
func (p *authorFormulaParser) take() string { t := p.peek(); p.pos = p.end; p.token = ""; return t }
func (p *authorFormulaParser) expression(min, depth int) bool {
	if depth > 32 {
		return false
	}
	t := p.take()
	if t == "" {
		return false
	}
	if t == "+" || t == "-" {
		if !p.expression(7, depth+1) {
			return false
		}
	} else if t == "(" {
		if !p.expression(0, depth+1) || p.take() != ")" {
			return false
		}
	} else {
		if p.peek() == "!" {
			p.take()
			t = p.take()
			if !authorCellRef.MatchString(t) {
				return false
			}
		}
		if p.peek() == "(" {
			p.take()
			count := 0
			if p.peek() != ")" {
				for {
					if !p.expression(0, depth+1) {
						return false
					}
					count++
					if count > 64 {
						return false
					}
					if p.peek() != "," {
						break
					}
					p.take()
				}
			}
			if p.take() != ")" || count == 0 {
				return false
			}
		} else if t[0] == '"' { // already checked for a closing quote
		} else if authorCellRef.MatchString(t) || strings.EqualFold(t, "TRUE") || strings.EqualFold(t, "FALSE") {
		} else if !authorNumber.MatchString(t) {
			return false
		}
	}
	for {
		op := p.peek()
		if op == "%" {
			p.take()
			continue
		}
		precedence := map[string]int{"=": 1, "<>": 1, "<": 1, ">": 1, "<=": 1, ">=": 1, "&": 2, "+": 3, "-": 3, "*": 4, "/": 4, "^": 5, ":": 6}[op]
		if precedence == 0 || precedence < min {
			break
		}
		p.take()
		next := precedence + 1
		if op == "^" {
			next = precedence
		}
		if !p.expression(next, depth+1) {
			return false
		}
	}
	return true
}
