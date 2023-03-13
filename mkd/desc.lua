local description = ""
local runes = 0

function get_description(blocks)
  for _, block in ipairs(blocks) do
    if block.t == 'Para' then
      description = pandoc.utils.stringify(block)
      break
    end
  end

  for _, block in ipairs(blocks) do
    if block.t == 'Para' then
      local s = pandoc.utils.stringify(block)
      s,_ = s:gsub("[a-zA-Z-]+","a")
      s,_ = s:gsub("%s+","")

      runes = runes + utf8.len(s)
    end
  end

  return nil
end

-- realpath convert path with .. to absolute path
--
-- "/1.md" == realpath("a.md", "1.md")
-- "/1.md" == realpath("/a.md", "1.md")
-- "/1.md" == realpath("/a.md", "./1.md")
-- "/1.md" == realpath("/a/b.md", "../1.md")
-- "/1.md" == realpath("/a/b/c.md", "../../1.md")
-- "/1.md" == realpath("/a/b/c/d.md", "../../../1.md")
function realpath(a, b)
  if a:sub(1,1) ~= '/' then a = '/' .. a end
  c = a:reverse()
  _,j = c:find("/")
  c = c:sub(j+1,-1)
  if b:sub(1,2) == "./" then
    b = b:sub(3)
  end
  while (true) do
    _,i = b:find("../")
    _,j = c:find("/")
    if i == nil or j == nil then break end
    b = b:sub(i,-1)
    c = c:sub(j+1,-1)
  end
  if b:sub(1,1) ~= '/' then b = '/' .. b end
  return c:reverse()..b
end

return {{
  Blocks = get_description,
  Meta = function (meta)
    meta.description = description
    meta.runes = string.format("%0.1f", runes/1000)
    meta.read_time = string.format("%0.1f", runes/400)
    local envs = pandoc.system.environment()
    for k,v in pairs(envs) do
      if meta[k] == nil then
        meta[k] = v
      end
    end
    return meta
  end,
  Image = function (img)
    img.attributes.loading = "lazy"
    return img
  end,
  Link = function (link)
    local t = link.target
    if t:sub(1,4) ~= "http" and t:sub(-3) == ".md" then
      local envs = pandoc.system.environment()
      local url = envs["site_url"]
      local p = PANDOC_STATE.input_files[1]
      t = realpath(p, t)
      t = url .. t
      link.target = t:sub(1,-3).."html"
    end
    return link
  end,
}}
