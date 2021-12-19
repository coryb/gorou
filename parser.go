package gorou

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var (
	goroutineRx = regexp.MustCompile(`\Agoroutine ([0-9]+) \[(.*?)(?:, (.*))?\]:\z`)
	// github.com/moby/buildkit/util/progress.(*progressReader).Read.func1()
	functionRx = regexp.MustCompile(`\A(created by )?((?:[a-zA-Z0-9.-]+/)?[a-zA-Z0-9/-]+)[.](.*)\z`)
	pathRx     = regexp.MustCompile(`\A\t(.*):([0-9]+)(?: (\+0x[0-9a-f]+))?\z`)
	ancestorRx = regexp.MustCompile(`\A\[originating from goroutine ([0-9]+)\]:\z`)
	argsRx     = regexp.MustCompile(`\(([^)]*)\)\z`)
	argRx      = regexp.MustCompile(`0x[a-f0-9]+`)
)

type GoRoutine struct {
	Num      int64
	Status   string
	Age      time.Duration
	Stack    Frames
	Ancestor *GoRoutine
}

func (gr *GoRoutine) String() string {
	//return fmt.Sprintf("%s", gr.Status)
	return fmt.Sprintf("[red]%-9d[-] - [::b]%s[::-] [blue]%s[-] [purple]%s[-]", gr.Num, gr.Status, gr.Age, gr.Stack.First().Short())
}

type Frame struct {
	Package      string
	Function     string
	Arguments    []uint64
	ArgumentsRaw string
	File         string
	Line         int64
}

func (f *Frame) Short() string {
	if f == nil {
		return ""
	}
	return f.Function
}

func (f *Frame) String() string {
	if f == nil {
		return ""
	}
	return fmt.Sprintf("%s.%s at %s:%d", f.Package, f.Function, f.File, f.Line)
}

type Frames []Frame

func (f Frames) First() *Frame {
	if len(f) == 0 {
		return nil
	}
	// TODO originally had the idea to show the first frame
	// that matches a given package, since that is more likely
	// to be useful for understanding your code pat
	return &f[0]
}

type GoRoutines []*GoRoutine

func (grs GoRoutines) ByAgeNum() GoRoutines {
	sort.Slice(grs, func(i, j int) bool {
		if grs[i].Age == grs[j].Age {
			return grs[i].Num < grs[j].Num
		}
		return grs[i].Age < grs[j].Age
	})
	return grs
}

func (grs GoRoutines) ByAgeGroups() []GoRoutines {
	ret := []GoRoutines{}
	sort.Slice(grs, func(i, j int) bool {
		return grs[i].Age < grs[j].Age
	})

	cursor := -1
	var lastAge *time.Duration
	for _, gr := range grs {
		if lastAge == nil || gr.Age != *lastAge {
			cursor++
			lastAge = &gr.Age
		}
		if len(ret) == cursor {
			ret = append(ret, GoRoutines{})
		}
		ret[cursor] = append(ret[cursor], gr)
	}

	return ret
}

func (grs GoRoutines) ByNum() GoRoutines {
	sort.Slice(grs, func(i, j int) bool {
		return grs[i].Num < grs[j].Num
	})
	return grs
}

func (grs GoRoutines) ByStatus() map[string]GoRoutines {
	ret := map[string]GoRoutines{}
	for _, gr := range grs {
		ret[gr.Status] = append(ret[gr.Status], gr)
	}
	return ret
}

type StackTrace struct {
	relatedArgs map[uint64]map[int64]*GoRoutine
	GoRoutines  GoRoutines
}

func NewStackTrace(fileName, filter string) (*StackTrace, error) {
	fh, err := os.Open(fileName)
	if err != nil {
		return nil, err
	}

	st := &StackTrace{
		relatedArgs: map[uint64]map[int64]*GoRoutine{},
	}

	var cursor *GoRoutine

	stackBuf := []string{}
	applyFilter := func() {
		if filter != "" && len(st.GoRoutines) > 0 {
			keep := false
			for _, l := range stackBuf {
				if strings.Contains(l, filter) {
					keep = true
					break
				}
			}
			if !keep {
				st.GoRoutines = st.GoRoutines[:len(st.GoRoutines)-1]
			}
		}
	}

	s := bufio.NewScanner(fh)
	for s.Scan() {
		line := s.Text()
		if line == "" {
			continue
		}
		if m := goroutineRx.FindStringSubmatch(line); m != nil {
			applyFilter()
			stackBuf = []string{line}
			num, err := strconv.ParseInt(m[1], 10, 64)
			if err != nil {
				return nil, fmt.Errorf("Failed to parse %s: %w", m[1], err)
			}
			// num, status, duration
			cursor = &GoRoutine{
				Num:    num,
				Status: m[2],
			}
			st.GoRoutines = append(st.GoRoutines, cursor)
			if m[3] != "" {
				cursor.Age, err = time.ParseDuration(strings.ReplaceAll(m[3], " minutes", "m"))
				if err != nil {
					return nil, fmt.Errorf("Failed to parse %s: %w", m[3], err)
				}
			}
			continue
		}
		stackBuf = append(stackBuf, line)
		if m := functionRx.FindStringSubmatch(line); m != nil {
			// created, package, function, arguments
			rawArgs := ""
			if argsMatch := argsRx.FindStringSubmatch(m[3]); argsMatch != nil {
				rawArgs = argsMatch[1]
			}
			funcName := argsRx.ReplaceAllString(m[3], "")
			sm := argRx.FindAllStringSubmatch(funcName, -1)
			args := []uint64{}
			for _, m := range sm {
				k, err := strconv.ParseUint(strings.TrimPrefix(m[0], "0x"), 16, 64)
				if err != nil {
					return nil, fmt.Errorf("Failed to parse %s: %w", m[0], err)
				}
				args = append(args, k)
				mp, ok := st.relatedArgs[k]
				if !ok {
					mp = map[int64]*GoRoutine{}
					st.relatedArgs[k] = mp
				}
				mp[cursor.Num] = cursor
			}

			cursor.Stack = append(cursor.Stack, Frame{
				Package:      m[2],
				Function:     funcName,
				Arguments:    args,
				ArgumentsRaw: rawArgs,
			})
			if !s.Scan() {
				return nil, fmt.Errorf("Invalid trace, expected %q at %q", pathRx, line)
			}
			line = s.Text()
			m = pathRx.FindStringSubmatch(line)
			if m == nil {
				return nil, fmt.Errorf("Invalid trace, %q does not match %q", line, pathRx)
			}
			// file, line, offset
			cursor.Stack[len(cursor.Stack)-1].File = m[1]
			num, err := strconv.ParseInt(m[2], 10, 64)
			if err != nil {
				return nil, fmt.Errorf("Failed to parse %s: %w", m[2], err)
			}
			cursor.Stack[len(cursor.Stack)-1].Line = num
			continue
		}
		if m := ancestorRx.FindStringSubmatch(line); m != nil {
			// num
			num, err := strconv.ParseInt(m[1], 10, 64)
			if err != nil {
				return nil, fmt.Errorf("Failed to parse %s: %w", m[1], err)
			}
			gr := &GoRoutine{
				Num: num,
			}
			cursor.Ancestor = gr
			cursor = gr
			continue
		}
		panic("Line Not Matched: " + line)
	}
	applyFilter()
	return st, nil
}
