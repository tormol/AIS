% Prepare binary windows release:
% UNTESTED
cp \r static release
%del release\robots.txt % not necessary for local use
cp README.md release\README.md
go build -o release\ais.exe server\*.go
zip release