package spf

import (
	"errors"
	"fmt"
	"net"
	"strings"
)

const ValidSPFVersion = "v=spf1"

type Result string

// Results defined as per https://www.rfc-editor.org/info/rfc7208/#section-2.6
const (
	NoneResult           Result = "none"
	NeutralResult        Result = "neutral"
	PassResult           Result = "pass"
	FailResult           Result = "fail"
	SoftFailResult       Result = "softfail"
	TempErrorResult      Result = "temperror"
	PermanentErrorResult Result = "permerror"
)

// VerifySPFHost, verifies a hostname's SPF records based on the RFC[1]
//
// [1] https://www.rfc-editor.org/info/rfc7208/#section-4
func VerifySPFHost(fromIP string, host string) (Result, error) {
	txtRecords, err := net.LookupTXT(host)
	if err != nil {
		fmt.Printf("Failed to lookup TXT records: %v\n", err)
		return TempErrorResult, err
	}

	// TODO: SPF record size check? https://www.rfc-editor.org/info/rfc7208/#section-3.4

	// selecting SPF records
	// https://www.rfc-editor.org/info/rfc7208/#section-4.5
	spfRecords := []string{}
	for _, txt := range txtRecords {
		if strings.HasPrefix(txt, fmt.Sprintf("%s ", ValidSPFVersion)) || txt == ValidSPFVersion {
			// spf records either have a space with more data, or end of record
			spfRecords = append(spfRecords, txt)
		}
	}

	if len(spfRecords) == 0 {
		return NoneResult, nil
	}
	if len(spfRecords) > 1 {
		return PermanentErrorResult, errors.New("More than one SPF record observed")
	}

	spfRecord := ParseRecord(spfRecords[0])

	for _, term := range spfRecord.DirectiveTerms {
		// Check and return based on result
		if term.mechanism != nil && term.mechanism.mechanismType == AllMechanismType {
			// since all is the last term, we can return here.
			return mapQualifierToResult(term.qualifier), nil
		}

		if term.mechanism != nil && term.mechanism.mechanismType == IP4MechanismType {
			// the IP put in may or may not contain a CIDR range, if not default to /32
			parsedFromIP := net.ParseIP(fromIP)

			// TODO: can panic
			spfIP := *term.mechanism.value
			if !strings.Contains(spfIP, "/") {
				// does not contain a CIDR range, default to /32
				spfIP += fmt.Sprintf("/32")
			}
			_, netRange, _ := net.ParseCIDR(spfIP)

			if netRange.Contains(parsedFromIP) {
				// Only if we match do we return the pass/fail result, else if no match go onto the next term.
				return mapQualifierToResult(term.qualifier), nil
			}
		}
	}

	return NoneResult, nil
}

func mapQualifierToResult(qualifier byte) Result {
	// https://www.rfc-editor.org/info/rfc7208/#section-4.6.2
	switch qualifier {
	case '+':
		return PassResult
	case '-':
		return FailResult
	case '~':
		return SoftFailResult
	case '?':
		return NeutralResult
	default:
		return PermanentErrorResult
	}
}
