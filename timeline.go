package gorou

import (
	"fmt"
	"path"
	"strconv"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type Timeline struct {
	*tview.TextView
	st           *StackTrace
	groupIx      int
	groupIxByID  map[string]int
	currentNumIx int
	depth        int
	timeline     []GoRoutines
	drawDetails  func(*GoRoutine)
	groupBy      GroupBy
	pkg          string
}

type GroupBy int

const (
	GroupByNone GroupBy = iota
	GroupByAge
	GroupByReturnAddress
)

func NewTimeline(st *StackTrace, groupBy GroupBy, pkg string, drawDetails func(*GoRoutine)) *Timeline {
	textView := tview.NewTextView().
		SetDynamicColors(true).
		SetRegions(true)
	textView.SetBorder(true)
	t := &Timeline{
		TextView:     textView,
		st:           st,
		groupIx:      0,
		groupIxByID:  make(map[string]int),
		currentNumIx: 0,
		drawDetails:  drawDetails,
		groupBy:      groupBy,
		pkg:          pkg,
	}

	switch groupBy {
	case GroupByAge:
		t.timeline = st.GoRoutines.ByAgeGroups()
		for i, group := range t.timeline {
			group.ByNum()
			t.groupIxByID[t.groupID(group)] = i
		}
	case GroupByReturnAddress:
		t.timeline = st.GoRoutines.ByReturnAddress()
		for i, group := range t.timeline {
			group.ByNum()
			t.groupIxByID[t.groupID(group)] = i
		}

	default:
		t.depth = 1
		t.timeline = []GoRoutines{
			st.GoRoutines.ByAgeNum(),
		}
	}

	textView.SetHighlightedFunc(t.highlightedFunc)
	t.highlight()
	return t
}

func (t *Timeline) highlightedFunc(added, removed, remaining []string) {
	addedGroup := withPrefix(added, "group:")
	if len(addedGroup) > 0 {
		t.groupIx = t.groupIxByID[addedGroup[0]]
		return
	}

	addedNum := withPrefix(added, "num:")
	if len(addedNum) > 0 {
		grNum, _ := strconv.ParseInt(addedNum[0], 10, 64)
		for i, gr := range t.timeline[t.groupIx] {
			if grNum == gr.Num {
				t.currentNumIx = i
				t.drawDetails(gr)
				return
			}
		}
	}
}

func (t *Timeline) Draw(screen tcell.Screen) {
	t.TextView.Clear()
	if t.depth > 0 {
		for _, gr := range t.timeline[t.groupIx] {
			f := gr.Stack.First()
			if t.pkg != "" {
				for _, fr := range gr.Stack {
					if strings.Contains(fr.Package, t.pkg) {
						f = &fr
						break
					}
				}
			}
			switch t.groupBy {
			case GroupByAge, GroupByReturnAddress:
				fmt.Fprintf(t, `➤ [red]["num:%d"]%-9d["-"][-] [::b]%s[::-] [purple]%s[-].[blue]%s[-]`+"\n", gr.Num, gr.Num, gr.Status, path.Base(f.Package), f.Function)
			default:
				fmt.Fprintf(t, `➤ [red]["num:%d"]%-9d["-"][-] [::b]%s[::-] [yellow]%s[-] [purple]%s[-].[blue]%s[-]`+"\n", gr.Num, gr.Num, gr.Status, gr.Age, path.Base(f.Package), f.Function)
			}
		}
	} else {
		for _, group := range t.timeline {
			fmt.Fprintf(t, `➤ ["group:%s"]%s[""] (%d)`+"\n", t.groupID(group), t.groupLabel(group), len(group))
		}
	}
	t.TextView.Draw(screen)
}

func (t *Timeline) groupID(group GoRoutines) string {
	switch t.groupBy {
	case GroupByAge:
		return group[0].Age.String()
	case GroupByReturnAddress:
		return group[0].AllReturnAddressesSHA
	default:
		return ""
	}
}

func (t *Timeline) groupLabel(group GoRoutines) string {
	switch t.groupBy {
	case GroupByAge:
		return group[0].Age.String()
	case GroupByReturnAddress:
		return fmt.Sprintf("%s %s", group[0].Stack.First().Function, "foo")
	default:
		return ""
	}
}

func withPrefix(s []string, prefix string) []string {
	ret := []string{}
	for _, e := range s {
		if strings.HasPrefix(e, prefix) {
			ret = append(ret, strings.TrimPrefix(e, prefix))
		}
	}
	return ret
}

func (t *Timeline) highlight() {
	if t.groupIx >= len(t.timeline) {
		return
	}
	switch t.depth {
	case 0:
		t.TextView.Highlight("group:" + t.groupID(t.timeline[t.groupIx]))
	case 1:
		if t.currentNumIx >= 0 && t.currentNumIx < len(t.timeline[t.groupIx]) {
			t.TextView.Highlight(fmt.Sprintf("num:%d", t.timeline[t.groupIx][t.currentNumIx].Num))
		}
	}
}

func (t *Timeline) InputHandler() func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
	return t.WrapInputHandler(func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
		if t.depth == 0 {
			switch event.Key() {
			case tcell.KeyUp:
				if t.groupIx > 0 {
					// toggle old one + new one
					t.highlight()
					t.groupIx--
					t.highlight()
				}
			case tcell.KeyDown:
				if t.groupIx < len(t.timeline)-1 {
					// toggle old one + new one
					t.highlight()
					t.groupIx++
					t.highlight()
				}
			case tcell.KeyEnter, tcell.KeyRight:
				t.depth = 1
				t.currentNumIx = 0
				t.highlight()
				t.ScrollToHighlight()
			}
		} else {
			switch event.Key() {
			case tcell.KeyUp:
				if t.currentNumIx > 0 {
					// toggle old one + new one
					t.highlight()
					t.currentNumIx--
					t.highlight()
				}
			case tcell.KeyDown:
				if t.currentNumIx < len(t.timeline[t.groupIx])-1 {
					// toggle old one + new one
					t.highlight()
					t.currentNumIx++
					t.highlight()
				}
			case tcell.KeyLeft:
				if t.groupBy > GroupByNone {
					t.depth = 0
					t.currentNumIx = -1
					t.highlight()
					t.ScrollToHighlight()
					t.drawDetails(nil)
				}
			}
		}
		t.TextView.InputHandler()(event, setFocus)
	})
}

func (t *Timeline) MouseHandler() func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (consumed bool, capture tview.Primitive) {
	return t.WrapMouseHandler(func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (consumed bool, capture tview.Primitive) {
		consumed, capture = t.TextView.MouseHandler()(action, event, setFocus)
		if consumed {
			t.depth = 1
			if t.currentNumIx == -1 {
				t.currentNumIx = 0
				t.highlight()
				t.ScrollToHighlight()
			}
		}
		return
	})
}
