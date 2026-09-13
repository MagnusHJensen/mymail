package spf

import (
	"net"
	"strings"
)

type SPFVersion string

const (
	Version1 SPFVersion = "spf1"
)

type SPFRecord struct {
	Version        string
	DirectiveTerms []directiveTerm
}

func ParseRecord(record string) SPFRecord {
	version, directiveTerms := readRecord(record)
	return SPFRecord{
		Version:        version,
		DirectiveTerms: directiveTerms,
	}
}

func readRecord(record string) (version string, directiveTerms []directiveTerm) {
	parts := strings.SplitN(record, " ", 2)
	version = parts[0]
	if len(parts) == 1 {
		// Record only contains version
		return version, nil
	}
	terms := strings.Split(parts[1], " ")

	directiveTerms = []directiveTerm{}
	for _, term := range terms {
		if isModifierTerm(term) {
			// modifier term
		} else {
			// directive term
			directiveTerm := parseDirectiveTerm(term)

			/* if directiveTerm.mechanism == nil {
				// fail each directive term requires a mechanism
				return "", nil // TODO: BETTER FAILURE
			} */

			directiveTerms = append(directiveTerms, directiveTerm)

			if directiveTerm.mechanism != nil && directiveTerm.mechanism.mechanismType == AllMechanismType {
				// https://www.rfc-editor.org/info/rfc7208/#section-5.1
				// short-circuit and ignore right of All mechanism
				// TODO: Figure out if that also counts for modifiers.
				return version, directiveTerms
			}
		}
	}

	return version, directiveTerms
}

func isModifierTerm(term string) bool {
	parts := strings.SplitN(term, "=", 2)
	return len(parts) == 2 // If we saw name=value, it's a modifier, else it's a directive
}

type directiveTerm struct {
	qualifier byte
	mechanism *mechanism
}

func parseDirectiveTerm(term string) directiveTerm {
	var qualifier byte
	if term[0] == '+' || term[0] == '-' || term[0] == '?' || term[0] == '~' {
		qualifier = term[0]
		term = term[1:]
	} else {
		// qualifier is optional and defaults to '+'
		qualifier = '+'
	}

	mechanism := parseMechanism(term)

	return directiveTerm{
		qualifier: qualifier,
		mechanism: mechanism,
	}
}

type MechanismType string

const (
	AllMechanismType     MechanismType = "all"
	IncludeMechanismType MechanismType = "include"
	AMechanismType       MechanismType = "a"
	MXMechanismType      MechanismType = "mx"
	// ptr has do not use in RFC
	IP4MechanismType    MechanismType = "ip4"
	IP6MechanismType    MechanismType = "ip6"
	ExistsMechanismType MechanismType = "exists"
)

type mechanism struct {
	mechanismType MechanismType
	value         *string
}

func parseMechanism(mechanismString string) *mechanism {
	switch {
	case strings.HasPrefix(mechanismString, string(AllMechanismType)):
		// handle all mechanism
		return &mechanism{
			mechanismType: AllMechanismType,
		}
	case strings.HasPrefix(mechanismString, string(IP4MechanismType)):
		parts := strings.Split(mechanismString, ":")

		// TODO: Handle malformed ip4

		ip := parts[1]
		parsedIP := net.ParseIP(ip)
		if parsedIP == nil {
			return nil // TODO: Better error
		}
		return &mechanism{
			mechanismType: IP4MechanismType,
			value:         &ip,
		}
	}

	return nil
}
