# vhs demo/desktop.tape — desktop-size screenshots + hero gif (run from repo root)
Output assets/paddock-demo.gif

Set Shell bash
Set FontSize 17
Set Width 1480
Set Height 620
Set Padding 12
Set Theme "TokyoNight"

Hide
Type "NO_COLOR= HERDR_BIN_PATH=$PWD/demo/mock-herdr ./paddock"
Enter
Sleep 5s
Show

Screenshot assets/paddock-wall.png
Sleep 1s
Down
Sleep 800ms
Down
Sleep 800ms
Right
Sleep 800ms
Enter
Sleep 2s
Screenshot assets/paddock-zoom.png
Sleep 1500ms
Escape
Sleep 1500ms

Hide
Type "q"
