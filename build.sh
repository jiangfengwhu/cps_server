CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o cps-go
scp cps-go root@vps:/root/workdir/cps
