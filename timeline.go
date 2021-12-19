package gorou

import (
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type Timeline struct {
	*tview.TextView
	st            *StackTrace
	times         []time.Duration
	currentTimeIx int
	currentNumIx  int
	depth         int
	timeline      []GoRoutines
	drawDetails   func(*GoRoutine)
	byAge         bool
}

func NewTimeline(st *StackTrace, byAge bool, drawDetails func(*GoRoutine)) *Timeline {
	textView := tview.NewTextView().
		SetDynamicColors(true).
		SetRegions(true)
	textView.SetBorder(true)
	depth := 0
	var timeline []GoRoutines
	times := []time.Duration{}
	if byAge {
		timeline = st.GoRoutines.ByAgeGroups()
		for _, group := range timeline {
			times = append(times, group[0].Age)
			group.ByNum()
		}
	} else {
		depth = 1
		timeline = []GoRoutines{
			st.GoRoutines.ByAgeNum(),
		}
	}

	t := &Timeline{
		TextView:      textView,
		st:            st,
		times:         times,
		currentTimeIx: 0,
		currentNumIx:  -1,
		depth:         depth,
		timeline:      timeline,
		drawDetails:   drawDetails,
		byAge:         byAge,
	}
	textView.SetHighlightedFunc(t.highlightedFunc)
	if byAge && len(times) > 0 {
		t.Highlight("age:" + times[0].String())
	} else if len(timeline) > 0 && len(timeline[0]) > 0 {
		t.Highlight(fmt.Sprintf("num:%d", timeline[0][0].Num))
	}
	return t
}

func (t *Timeline) highlightedFunc(added, removed, remaining []string) {
	addedAge := withPrefix(added, "age:")
	if len(addedAge) > 0 {
		for i, age := range t.times {
			if addedAge[0] == "age:"+age.String() {
				t.currentTimeIx = i
				return
			}
		}
	}
	addedNum := withPrefix(added, "num:")
	if len(addedNum) > 0 {
		for i, gr := range t.timeline[t.currentTimeIx] {
			if addedNum[0] == fmt.Sprintf("num:%d", gr.Num) {
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
		for _, gr := range t.timeline[t.currentTimeIx] {
			f := gr.Stack.First()
			if t.byAge {
				fmt.Fprintf(t, `➤ [red]["num:%d"]%-9d["-"][-] [::b]%s[::-] [purple]%s[-].[blue]%s[-]`+"\n", gr.Num, gr.Num, gr.Status, path.Base(f.Package), f.Function)
			} else {
				fmt.Fprintf(t, `➤ [red]["num:%d"]%-9d["-"][-] [::b]%s[::-] [yellow]%s[-] [purple]%s[-].[blue]%s[-]`+"\n", gr.Num, gr.Num, gr.Status, gr.Age, path.Base(f.Package), f.Function)
			}
		}
	} else {
		for _, group := range t.timeline {
			age := group[0].Age.String()
			fmt.Fprintf(t, `➤ ["age:%s"]%s[""] (%d)`+"\n", age, age, len(group))
		}
	}
	t.TextView.Draw(screen)
}

func withPrefix(s []string, prefix string) []string {
	ret := []string{}
	for _, e := range s {
		if strings.HasPrefix(e, prefix) {
			ret = append(ret, e)
		}
	}
	return ret
}

func (t *Timeline) InputHandler() func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
	return t.WrapInputHandler(func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
		if t.depth == 0 {
			switch event.Key() {
			case tcell.KeyUp:
				if t.currentTimeIx > 0 {
					// toggle old one + new one
					t.TextView.Highlight("age:" + t.times[t.currentTimeIx].String())
					t.currentTimeIx--
					t.TextView.Highlight("age:" + t.times[t.currentTimeIx].String())
				}
			case tcell.KeyDown:
				if t.currentTimeIx < len(t.times)-1 {
					// toggle old one + new one
					t.TextView.Highlight("age:" + t.times[t.currentTimeIx].String())
					t.currentTimeIx++
					t.TextView.Highlight("age:" + t.times[t.currentTimeIx].String())
				}
			case tcell.KeyEnter, tcell.KeyRight:
				t.depth = 1
				t.currentNumIx = 0
				gr := t.timeline[t.currentTimeIx][t.currentNumIx]
				t.Highlight(fmt.Sprintf("num:%d", gr.Num))
				t.ScrollToHighlight()
			}
		} else {
			switch event.Key() {
			case tcell.KeyUp:
				if t.currentNumIx > 0 {
					// toggle old one + new one
					t.TextView.Highlight(fmt.Sprintf("num:%d", t.timeline[t.currentTimeIx][t.currentNumIx].Num))
					t.currentNumIx--
					t.TextView.Highlight(fmt.Sprintf("num:%d", t.timeline[t.currentTimeIx][t.currentNumIx].Num))
				}
			case tcell.KeyDown:
				if t.currentNumIx < len(t.timeline[t.currentTimeIx])-1 {
					// toggle old one + new one
					t.TextView.Highlight(fmt.Sprintf("num:%d", t.timeline[t.currentTimeIx][t.currentNumIx].Num))
					t.currentNumIx++
					t.TextView.Highlight(fmt.Sprintf("num:%d", t.timeline[t.currentTimeIx][t.currentNumIx].Num))
				}
			case tcell.KeyLeft:
				if t.byAge {
					t.depth = 0
					t.currentNumIx = -1
					t.TextView.Highlight("age:" + t.times[t.currentTimeIx].String())
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
				gr := t.timeline[t.currentTimeIx][t.currentNumIx]
				t.Highlight(fmt.Sprintf("num:%d", gr.Num))
				t.ScrollToHighlight()
			}
		}
		return
	})
}
