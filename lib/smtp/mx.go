package smtp

import (
	"fmt"
	"net"
)

func GetPreferredMXHost(hostName string) (string, error) {
	mxRecords, err := net.LookupMX(hostName)
	if err != nil {
		return "", fmt.Errorf("Issue processing mail: %v\n", err)
	}

	if len(mxRecords) < 1 {
		return "", fmt.Errorf("NO error, but no MX records found.\n")
	}

	record := mxRecords[0]
	return record.Host, nil
}
