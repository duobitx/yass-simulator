package validation

import (
	"fmt"
	"regexp"

	"github.com/m-szalik/goutils"
)

var tleLine1Re = regexp.MustCompile(`^1\s(\d{5})([A-Z])\s(\d{2})(\d{3})([A-Z0-9 ]{1,3})\s(\d{2})(\d{3}\.\d{8})\s([+\-]\.\d{8})\s([0-9]{5}-[0-9])\s([+\-]?[0-9]{5}-[0-9])\s([0-9])\s([0-9]{4})([0-9])$`)
var tleLine2Re = regexp.MustCompile(`^2\s(\d{5})\s([0-9]{2}\.[0-9]{4})\s([0-9]{3}\.[0-9]{4})\s([0-9]{7})\s([0-9]{3}\.[0-9]{4})\s([0-9]{3}\.[0-9]{4})\s([0-9]{2}\.[0-9]{8})\s([0-9]{5})([0-9])$`)

func ValidateTLE(tle []string, elementIndex int, jeh *goutils.JoinErrorHelper) {
	l := len(tle)
	switch l {
	case 2:
	default:
		jeh.Append(fmt.Errorf("element %d: TLE must contains 2 lines, but this one has %d lines", elementIndex, l))
		return
	}
	if !tleLine1Re.MatchString(tle[0]) {
		jeh.Append(fmt.Errorf("element %d: 1st line of TLE is invalid", elementIndex))
	}
	if !tleLine2Re.MatchString(tle[0]) {
		jeh.Append(fmt.Errorf("element %d: 2nd line of TLE is invalid", elementIndex))
	}
}
