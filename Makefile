build:
	vulcanize --abspath . --strip-comments --inline-scripts --inline-css static/app.html > static/index.html

deps:
	sudo npm install -g vulcanize

clean:
	rm -f static/index.html

.PHONY: build clean
