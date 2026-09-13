package spf

import (
	"errors"
	"fmt"
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

func ParseRecord(record string) (*SPFRecord, error) {
	version, directiveTerms, err := readRecord(record)
	if err != nil {
		return nil, err
	}
	return &SPFRecord{
		Version:        version,
		DirectiveTerms: directiveTerms,
	}, nil
}

func readRecord(record string) (version string, directiveTerms []directiveTerm, err error) {
	parts := strings.SplitN(record, " ", 2)
	version = parts[0]
	if len(parts) == 1 {
		// Record only contains version
		return version, nil, nil
	}
	terms := strings.Split(parts[1], " ")

	directiveTerms = []directiveTerm{}
	for _, term := range terms {
		if isModifierTerm(term) {
			// modifier term
		} else {
			// directive term
			directiveTerm, err := parseDirectiveTerm(term)
			if err != nil {
				return "", nil, err
			}

			if directiveTerm.mechanism == nil {
				// fail each directive term requires a mechanism
				return "", nil, errors.New("failed to parse mechanism for term")
			}

			directiveTerms = append(directiveTerms, *directiveTerm)

			if directiveTerm.mechanism != nil && directiveTerm.mechanism.mechanismType == AllMechanismType {
				// https://www.rfc-editor.org/info/rfc7208/#section-5.1
				// short-circuit and ignore right of All mechanism
				// TODO: Figure out if that also counts for modifiers.
				return version, directiveTerms, nil
			}
		}
	}

	return version, directiveTerms, nil
}

func isModifierTerm(term string) bool {
	parts := strings.SplitN(term, "=", 2)
	return len(parts) == 2 // If we saw name=value, it's a modifier, else it's a directive
}

type directiveTerm struct {
	qualifier byte
	mechanism *mechanism
}

func parseDirectiveTerm(term string) (*directiveTerm, error) {
	var qualifier byte
	if term[0] == '+' || term[0] == '-' || term[0] == '?' || term[0] == '~' {
		qualifier = term[0]
		term = term[1:]
	} else {
		// qualifier is optional and defaults to '+'
		qualifier = '+'
	}

	mechanism, err := parseMechanism(term)
	if err != nil {
		return nil, err
	}

	return &directiveTerm{
		qualifier: qualifier,
		mechanism: mechanism,
	}, nil
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

func parseMechanism(mechanismString string) (*mechanism, error) {
	if len(mechanismString) == 0 {
		return nil, errors.New("mechanism can not be an empty string")
	}

	parts := strings.SplitN(mechanismString, ":", 2)
	mechType := parts[0]

	switch mechType {
	case string(AllMechanismType):
		// handle all mechanism
		return &mechanism{
			mechanismType: AllMechanismType,
		}, nil
	case string(IP4MechanismType), string(IP6MechanismType):
		return parseIPMechanism(parts, mechType)
	}

	return nil, fmt.Errorf("unsupported mechanism: %s", mechType)
}

func parseIPMechanism(parts []string, mechType string) (*mechanism, error) {
	ipType := IP4MechanismType
	if mechType == string(IP6MechanismType) {
		ipType = IP6MechanismType
	}

	if len(parts) < 2 {
		return nil, fmt.Errorf(`%s mechanism requires a value separated by ":"`, ipType)
	}

	var ipValue string

	ip := parts[1]
	if strings.Contains(ip, "/") {
		// CIDR address
		_, ipNet, err := net.ParseCIDR(ip)
		if err != nil {
			return nil, fmt.Errorf("failed to parse %s value: %w", ipType, err)
		}

		ipValue = ipNet.String()
	} else {
		// parse as regular IP without CIDR
		parsedIP := net.ParseIP(ip)
		if parsedIP == nil {
			return nil, fmt.Errorf("failed to parse %s value", ipType)
		}
		ipValue = parsedIP.String()
	}
	return &mechanism{
		mechanismType: ipType,
		value:         &ipValue,
	}, nil
}
