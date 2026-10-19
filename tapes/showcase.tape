# Outfitter deterministic showcase.
# Record from the repository root with:
#   make build && vhs tapes/showcase.tape
Output docs/media/showcase.gif

Set Shell "bash"
Set FontSize 16
Set FontFamily "DejaVu Sans Mono"
Set Width 1440
Set Height 900
Set Padding 16
Set Framerate 12
Set CursorBlink false
Set Theme { "background": "#0f172a", "foreground": "#e2e8f0", "cursor": "#38bdf8" }

Hide
Type "./outfitter --demo"
Enter
Sleep 1s
Show
Sleep 2s

# Browse tools and visit categories.
Type "j"
Sleep 600ms
Type "j"
Sleep 600ms
Type "k"
Sleep 500ms
Type "2"
Sleep 800ms
Type "3"
Sleep 800ms
Type "1"
Sleep 800ms

# Cycle preset profiles, then queue all available updates.
Type "]"
Sleep 1s
Type "]"
Sleep 1s
Type "u"
Sleep 1s

# Review the dry-run plan, run the simulated execution, and view the summary.
Enter
Sleep 2s
Enter
Sleep 3s
Escape
Sleep 1200ms

# Inspect help, cycle themes, then quit cleanly.
Type "?"
Sleep 1800ms
Escape
Sleep 600ms
Type "t"
Sleep 900ms
Type "t"
Sleep 900ms
Hide
Type "q"
Sleep 500ms
