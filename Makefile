build:
	vulcanize --abspath . --strip-comments --inline-scripts --inline-css static/app.html > static/index.html

clean:
	rm -f static/index.html

.PHONY: build clean
