package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/coryb/gorou"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"gopkg.in/alecthomas/kingpin.v2"
)

var (
	dirs = map[string]string{}
)

func main() {
	var traceFile, pkg string
	var filters, excludes []string
	var age bool
	ancestorFrames := -1
	kingpin.Flag("dir", "Map file paths").Short('d').StringMapVar(&dirs)
	kingpin.Flag("filter", "Exclude stacks missing value").Short('f').StringsVar(&filters)
	kingpin.Flag("exclude", "Exclude stacks with value").Short('x').StringsVar(&excludes)
	kingpin.Flag("pkg", "Package to focus").Short('p').StringVar(&pkg)
	kingpin.Flag("age", "Display goroutines grouped by age").BoolVar(&age)
	kingpin.Flag("ancestor-frames", "Limit frames for each ancestor").Short('A').IntVar(&ancestorFrames)
	kingpin.Arg("TRACE FILE", "Path to trace file").Required().StringVar(&traceFile)
	kingpin.Parse()

	st, err := gorou.NewStackTrace(traceFile, filters, excludes)
	noErr(err, traceFile)

	app := tview.NewApplication()

	topText := tview.NewTextView().
		SetDynamicColors(true).SetWrap(false).SetRegions(true)
	topText.SetBorder(true)

	bottomText := tview.NewTextView().
		SetDynamicColors(true).SetWrap(false).SetRegions(true)
	bottomText.SetBorder(true)

	tree := gorou.NewTimeline(st, age, pkg, func(gr *gorou.GoRoutine) {
		drawStack(gr, topText, filters)
		drawAncestors(gr, bottomText, ancestorFrames, filters)
	})

	focus := 0
	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyTab {
			focus = (focus + 1) % 3
			switch focus {
			case 0:
				app.SetFocus(tree)
			case 1:
				app.SetFocus(topText)
			case 2:
				app.SetFocus(bottomText)
			}
		}
		return event
	})

	flex := tview.NewFlex().
		AddItem(tree, 0, 40, true).
		AddItem(
			tview.NewFlex().SetDirection(tview.FlexRow).AddItem(
				topText, 0, 50, false,
			).AddItem(
				bottomText, 0, 50, false,
			), 0, 60, false,
		)

	if err := app.SetRoot(flex, true).EnableMouse(true).Run(); err != nil {
		panic(err)
	}
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

func noErr(err error, context string) {
	if err != nil {
		log.Print(context)
		panic(err)
	}
}

func drawStack(gr *gorou.GoRoutine, panel *tview.TextView, filters []string) {
	panel.Clear()
	if gr == nil {
		panel.SetTitle("")
		return
	}
	panel.SetTitle(fmt.Sprintf(" goroutine %d ", gr.Num))
	for _, f := range gr.Stack {
		msg := fmt.Sprintf("[purple]%s[-].[blue]%s[-](%s)\n\t%s:%d\n", f.Package, f.Function, f.ArgumentsRaw, filename(f.File), f.Line)
		for _, filter := range filters {
			msg = strings.ReplaceAll(msg, filter, `["`+filter+`"]`+filter+`[""]`)
		}

		fmt.Fprintf(panel, msg)
	}
	for _, filter := range filters {
		panel.Highlight(filter)
	}
	panel.ScrollToBeginning()
}

func drawAncestors(gr *gorou.GoRoutine, panel *tview.TextView, frames int, filters []string) {
	panel.Clear()
	if gr == nil {
		panel.SetTitle("")
		return
	}
	panel.SetTitle(fmt.Sprintf(" ancestors of %d ", gr.Num))
	for gr != nil && gr.Ancestor != nil {
		gr = gr.Ancestor
		fmt.Fprintf(panel, `➤ [red]%-9d[-]`+"\n", gr.Num)
		for i, f := range gr.Stack {
			if frames >= 0 && i >= frames {
				break
			}
			msg := fmt.Sprintf("  [purple]%s[-].[blue]%s[-](%s)\n\t%s:%d\n", f.Package, f.Function, f.ArgumentsRaw, filename(f.File), f.Line)
			for _, filter := range filters {
				msg = strings.ReplaceAll(msg, filter, `["`+filter+`"]`+filter+`[""]`)
			}

			fmt.Fprintf(panel, msg)
		}
	}
	for _, filter := range filters {
		panel.Highlight(filter)
	}
	panel.ScrollToBeginning()
}

func filename(fn string) string {
	for src, dest := range dirs {
		if strings.HasPrefix(fn, src) {
			fn = filepath.Join(dest, strings.TrimPrefix(fn, src))
		}
	}
	if strings.HasPrefix(fn, os.Getenv("HOME")) {
		fn = filepath.Join("~", strings.TrimPrefix(fn, os.Getenv("HOME")))
	}
	return fn
}
