echo "Splitting $1"
pages=$(pdfinfo $1 | grep Pages | awk '{print $2}')
dir=$(dirname $1)
building=$(basename $dir)
echo "Pages: $pages"

for i in `seq 1 $pages`;
do
  out="$dir/${building}_floor$i.svg"
  echo $out
  pdf2svg $1 $out $i
done
