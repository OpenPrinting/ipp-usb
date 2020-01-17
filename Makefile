all:
	-gotags -R . > tags
	go build -ldflags "-s -w"
