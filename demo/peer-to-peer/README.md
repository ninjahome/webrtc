#### Linux/macOS
Run ` ./offer `
send $BROWSER_SDP to answer on anther pc

#### Linux/macOS
Run `echo $BROWSER_SDP | ./answer `

copy   $BROWSER_SDP and send it to offer's pc
curl localhost:50000/sdp -d $BROWSER_SDP