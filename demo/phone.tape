# vhs demo/phone.tape — a 44-column portrait terminal, like Termux over SSH
Output assets/paddock-phone-throwaway.gif

Set Shell bash
Set FontSize 20
Set Width 560
Set Height 1220
Set Padding 8
Set Theme "TokyoNight"

Hide
Type "NO_COLOR= HERDR_BIN_PATH=$PWD/demo/mock-herdr ./paddock"
Enter
Sleep 5s
Show

Screenshot assets/paddock-phone-wall.png
Sleep 500ms
Enter
Sleep 2s
Screenshot assets/paddock-phone-zoom.png
Sleep 500ms

Hide
Escape
Type "q"
