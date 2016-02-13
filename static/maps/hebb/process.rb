def process file, args, out
  puts "Processing #{file}"
  # -rotate 28
  system("convert -density 150 #{args} -trim -background white -alpha remove -fuzz 5% -fill none -floodfill +0+0 white  -trim #{file} #{out}")
end

process("hebb_basement.pdf", "", "hebb_intermediate_0000.png")
process("hebb_groundfloor.pdf", "", "hebb_intermediate_0001.png")
process("hebb_secondfloor.pdf", "", "hebb_intermediate_0002.png")
process("hebb_thirdfloor.pdf", "", "hebb_intermediate_0003.png")
process("hebb_fourthfloor.pdf", "", "hebb_intermediate_0004.png")
process("hebb_fifthfloor.pdf", "", "hebb_intermediate_0005.png")
process("hebb_penthouse.pdf", "", "hebb_intermediate_0006.png")
