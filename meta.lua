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
    -- 触发脚本为 index.sh . 所以路径以 . 开头
    meta.path = PANDOC_STATE.input_files[1]:sub(2,-3).."html"
    return meta
  end,
}}
