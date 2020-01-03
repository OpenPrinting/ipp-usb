all:
	gotags -R . > tags
	go build
