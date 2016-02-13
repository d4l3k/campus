4.times do |i|
  system("convert -background 'rgba(0,0,0,0)' -rotate 152 -trim mcld_000#{i+1}.png mcld_rotated_000#{i+1}.png")
end
