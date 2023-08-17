#### Linux/macOS
Run ` ./read-play `
send $BROWSER_SDP 

Run `echo $BROWSER_SDP | ./offer `

copy   $BROWSER_SDP and send it to read-play

`curl localhost:50000/sdp -d $BROWSER_SDP`