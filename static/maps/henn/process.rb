def process file, args, out
  puts "Processing #{file}"
  system("convert -density 150 #{args} -trim -background white -alpha remove -rotate 332 -fuzz 5% -fill none -floodfill +0+0 white  -trim #{file} #{out}")
end

process("hennings_firstfloor.pdf", "-crop 1200x862+215+148", "henn_0001.png")
process("hennings_secondfloor.pdf", "-crop 1200x862+215+148", "henn_0002.png")
process("hennings_mezzanine.pdf", "-crop 1200x862+215+148", "henn_0003.png")
process("hennings_thirdfloor.pdf", "-crop 1200x862+215+148", "henn_0004.png")
process("hennings_penthouses.pdf", "-crop 1200x862+215+148", "henn_0005.png")
