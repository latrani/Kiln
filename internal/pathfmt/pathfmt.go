// Package pathfmt fills in the file and folder name templates in Kiln's
// settings (log_dir, log_name, export_name): strftime-style verbs for
// when (%Y, %m, %d, %H, %M, %S, …) and {braces} for who ({world},
// {char}, {name}, …).
package pathfmt

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
)

// verbs are the strftime verbs a template may use, as Go layouts.
var verbs = map[byte]string{
	'Y': "2006", // year
	'y': "06",   // year, two digits
	'm': "01",   // month, 01-12
	'b': "Jan",  // month name, short
	'B': "January",
	'd': "02",  // day of the month, 01-31
	'a': "Mon", // weekday, short
	'A': "Monday",
	'H': "15", // hour, 00-23
	'I': "03", // hour, 01-12
	'p': "PM",
	'M': "04", // minute
	'S': "05", // second
}

var varRE = regexp.MustCompile(`\{([^{}]*)\}`)

// Check reports whether template uses only the given {vars} and known
// %verbs.
func Check(template string, vars []string) error {
	for _, m := range varRE.FindAllStringSubmatch(template, -1) {
		if !slices.Contains(vars, m[1]) {
			return fmt.Errorf("unknown placeholder {%s} (use {%s})", m[1], strings.Join(vars, "}, {"))
		}
	}
	for i := 0; i < len(template); i++ {
		if template[i] != '%' {
			continue
		}
		if i+1 == len(template) {
			return fmt.Errorf("%% at the end (use %%%% for a %%)")
		}
		if c := template[i+1]; c != '%' && verbs[c] == "" {
			return fmt.Errorf("unknown time code %%%c", c)
		}
		i++
	}
	return nil
}

// Expand fills in template: each {var} from vars (unknown ones are left
// as they are), and each %verb from t. Values are made safe for a path
// component: "/" and "\" become "_".
func Expand(template string, vars map[string]string, t time.Time) string {
	s := varRE.ReplaceAllStringFunc(template, func(m string) string {
		v, ok := vars[m[1:len(m)-1]]
		if !ok {
			return m
		}
		return strings.NewReplacer("/", "_", `\`, "_").Replace(v)
	})
	return Strftime(s, t)
}

// Strftime fills in the %verbs of s from t; %% is a literal %.
func Strftime(s string, t time.Time) string {
	if !strings.Contains(s, "%") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '%' || i+1 == len(s) {
			b.WriteByte(s[i])
			continue
		}
		i++
		if layout := verbs[s[i]]; layout != "" {
			b.WriteString(t.Format(layout))
		} else {
			b.WriteByte(s[i]) // %% and unknown verbs
		}
	}
	return b.String()
}

// HasTime reports whether template uses any %verb.
func HasTime(template string) bool {
	return strings.Contains(strings.ReplaceAll(template, "%%", ""), "%")
}
