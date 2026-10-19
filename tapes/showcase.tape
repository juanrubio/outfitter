# Outfitter deterministic showcase.
# Record with: vhs tapes/showcase.tape
# For an MP4 artifact, replace the Output line with: Output tapes/showcase.mp4
Output tapes/showcase.gif

Set Shell "bash"
Set FontSize 16
Set Width 1440
Set Height 900
Set Theme "Catppuccin Mocha"
Set Padding 16
Set MarginFill "#0b1020"
Set CursorBlink false
Set TypingSpeed 40ms

Hide
Type "./outfitter --demo"
Enter
Show
Sleep 2s

# Browse tools and visit each category.
Type "j"
Sleep 300ms
Type "k"
Sleep 300ms
Type "1"
Sleep 600ms
Type "j"
Sleep 300ms
Type "2"
Sleep 600ms
Type "3"
Sleep 600ms
Type "4"
Sleep 600ms

# Cycle preset profiles, then queue all available updates.
Type "]"
Sleep 900ms
Type "]"
Sleep 900ms
Type "u"
Sleep 900ms

# Review the dry-run plan, run the simulated execution, and view the summary.
Type "Enter"
Sleep 1500ms
Type "Enter"
Sleep 2500ms
Type "Esc"
Sleep 1200ms

# Inspect help, cycle themes, then quit cleanly.
Type "?"
Sleep 1500ms
Type "Esc"
Sleep 500ms
Type "t"
Sleep 800ms
Type "t"
Sleep 800ms
Type "t"
Sleep 800ms
Type "q"
Sleep 1200ms
