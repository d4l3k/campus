system("convert -background 'rgba(0,0,0,0)' -rotate -28 -trim kais_0001.png kais_rotated_0001.png")
4.times do |i|
  system("convert -background 'rgba(0,0,0,0)' -rotate 62 -trim kais_000#{i+2}.png kais_rotated_000#{i+2}.png")
end
