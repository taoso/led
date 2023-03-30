local description = ""

function get_description(blocks)
  for _, block in ipairs(blocks) do
    if block.t == 'Para' then
      description = pandoc.utils.stringify(block)
      break
    end
  end
  return nil
end

return {{
  Blocks = get_description,
  Meta = function (meta)
    meta.desc = description
    meta.path = "/"..PANDOC_STATE.input_files[1]:sub(1,-3).."html"
    return meta
  end,
}}
