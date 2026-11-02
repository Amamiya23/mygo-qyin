package control

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// CLI runs `lilt ctl <command> [args]` and returns the exit code.
func CLI(args []string) int {
	return cli(args, os.Stdout, os.Stderr)
}

func cli(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		if len(args) > 1 {
			if cmd := lookup(args[1]); cmd != nil {
				commandHelp(stdout, cmd)
				return 0
			}
		}
		usage(stdout)
		return 0
	}
	cmd := lookup(args[0])
	if cmd == nil {
		fmt.Fprintf(stderr, "unknown command %q\n\n", args[0])
		usage(stderr)
		return 2
	}
	values, err := parseArgs(cmd, args[1:])
	if err == errHelp {
		commandHelp(stdout, cmd)
		return 0
	}
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n\n", cmd.Name, err)
		commandHelp(stderr, cmd)
		return 2
	}
	raw, _ := json.Marshal(values)
	data, err := Call(context.Background(), cmd.Name, raw)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	var out bytes.Buffer
	if json.Indent(&out, data, "", "  ") != nil {
		out.Write(data)
	}
	out.WriteByte('\n')
	stdout.Write(out.Bytes())
	return 0
}

var errHelp = fmt.Errorf("help")

// parseArgs reads `--name value`, `--name=value`, bare `--flag` for
// booleans, and positional values for the required parameters in order.
func parseArgs(cmd *Command, args []string) (map[string]any, error) {
	values := map[string]any{}
	var positional []Param
	for _, p := range cmd.Params {
		if p.Required {
			positional = append(positional, p)
		}
	}
	flags := true
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if flags && arg == "--" {
			flags = false
			continue
		}
		if flags && (arg == "-h" || arg == "--help") {
			return nil, errHelp
		}
		if !flags || !strings.HasPrefix(arg, "--") {
			if len(positional) == 0 {
				return nil, fmt.Errorf("unexpected argument %q", arg)
			}
			p := positional[0]
			positional = positional[1:]
			if _, set := values[p.Name]; set {
				return nil, fmt.Errorf("%s given twice", p.Name)
			}
			v, err := convert(p, arg)
			if err != nil {
				return nil, err
			}
			values[p.Name] = v
			continue
		}
		name, val, hasVal := strings.Cut(strings.TrimPrefix(arg, "--"), "=")
		name = strings.ReplaceAll(name, "-", "_")
		var p *Param
		for j := range cmd.Params {
			if cmd.Params[j].Name == name {
				p = &cmd.Params[j]
			}
		}
		if p == nil {
			return nil, fmt.Errorf("unknown flag --%s", name)
		}
		if !hasVal {
			if p.Type == "boolean" && (i+1 >= len(args) || !isBool(args[i+1])) {
				val = "true"
			} else if i+1 < len(args) && (p.Type != "boolean" || isBool(args[i+1])) {
				i++
				val = args[i]
			} else {
				return nil, fmt.Errorf("--%s needs a value", name)
			}
		}
		v, err := convert(*p, val)
		if err != nil {
			return nil, err
		}
		values[p.Name] = v
		positional = removeParam(positional, p.Name)
	}
	for _, p := range cmd.Params {
		if _, set := values[p.Name]; p.Required && !set {
			return nil, fmt.Errorf("missing %s", p.Name)
		}
	}
	return values, nil
}

func isBool(s string) bool { _, err := strconv.ParseBool(s); return err == nil }

func removeParam(ps []Param, name string) []Param {
	out := ps[:0:0]
	for _, p := range ps {
		if p.Name != name {
			out = append(out, p)
		}
	}
	return out
}

func convert(p Param, s string) (any, error) {
	switch p.Type {
	case "integer":
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("%s: %q is not an integer", p.Name, s)
		}
		return n, nil
	case "number":
		n, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return nil, fmt.Errorf("%s: %q is not a number", p.Name, s)
		}
		return n, nil
	case "boolean":
		b, err := strconv.ParseBool(s)
		if err != nil {
			return nil, fmt.Errorf("%s: %q is not true or false", p.Name, s)
		}
		return b, nil
	case "integers":
		var ids []int64
		for f := range strings.SplitSeq(s, ",") {
			n, err := strconv.ParseInt(strings.TrimSpace(f), 10, 64)
			if err != nil {
				return nil, fmt.Errorf("%s: %q is not a comma-separated list of integers", p.Name, s)
			}
			ids = append(ids, n)
		}
		return ids, nil
	}
	if len(p.Enum) > 0 && !contains(p.Enum, s) {
		return nil, fmt.Errorf("%s must be one of %s", p.Name, strings.Join(p.Enum, ", "))
	}
	return s, nil
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func usage(w io.Writer) {
	fmt.Fprint(w, `Control the running Lilt app. Output is JSON.

Usage: lilt ctl <command> [args]
       lilt ctl help <command>

Commands:
`)
	for _, c := range Commands {
		fmt.Fprintf(w, "  %-16s %s\n", c.Name, c.Help)
	}
}

func commandHelp(w io.Writer, c *Command) {
	var line strings.Builder
	line.WriteString("lilt ctl " + c.Name)
	for _, p := range c.Params {
		if p.Required {
			line.WriteString(" <" + p.Name + ">")
		}
	}
	for _, p := range c.Params {
		if !p.Required {
			line.WriteString(" [--" + p.Name + " " + placeholder(p) + "]")
		}
	}
	fmt.Fprintf(w, "%s\n\n%s\n", line.String(), c.Help)
	if len(c.Params) > 0 {
		fmt.Fprintln(w)
	}
	for _, p := range c.Params {
		help := p.Help
		if len(p.Enum) > 0 {
			help = strings.TrimSpace(help + " [" + strings.Join(p.Enum, "|") + "]")
		}
		fmt.Fprintf(w, "  --%-12s %s\n", p.Name, help)
	}
}

func placeholder(p Param) string {
	switch {
	case p.Type == "integers":
		return "1,2,3"
	case p.Type == "boolean":
		return "true|false"
	case len(p.Enum) > 0:
		return strings.Join(p.Enum, "|")
	}
	return p.Type
}
