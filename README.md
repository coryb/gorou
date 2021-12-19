# gorou

This is an experimental command line TUI to help parse and analyze goroutine stack trace files.  You can sort the stacks by age and filter stacks that match
a string.  You can also remap paths from the stack back to local source for easier integration to jump to the source (on iTerm you can use CMD+LeftClick to make source paths open in your default editor).

Three panes are displayed.  The left pane is a list of goroutines to analyze with the function at the top of the stack listed.  The top right pane is the full stack for the currently selected goroutine3 from the left pane.  The bottom right pane is for ancestors of the currently selected goroutine.  For the ancestor pane to be useful, it is recommended to run you Go program with something like `export GODEBUG=tracebackancestors=5`
