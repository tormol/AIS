A program that can merge AIS messages from multiple sources, repeat the merged stream and display ships on a map

The merged stream can be received as AIS sentences over

* HTTP: `wget -qO- localhost/api/v1/raw`
* TCP: `nc localhost 23` or `telnet localhost`
* UDP: `nc -u localhost 23` and press enter every few seconds.

# Invocation

`./bin [-port-prefix=NN] [name(:timeout)=URL ...]`
If no servers are listed, it will use http://aishub.ais.ecc.no/raw and tcp://153.44.253.27:5631.

The name is used in error messages and logged statistics.  
The timeout is per packet and must have an unit such as `s`, `ms` and `ns`.  
The URL can be `http://`, `tcp://` or `file://`, when it's a file the program
will terminate after the end of file is reached.

`-port-prefix` is an offset to the listening port numbers, multiplied by 100.  
The default value is 80, which means the server listen on :8023 for TCP and UDP forwarding, and :8080 for web. Changing the port is necessary to run two servers in paralell.

## Example
Start a second server that reads from one already running:
`go run server/*go -port-offset=81 other:5s=tcp://localhost:8023`
