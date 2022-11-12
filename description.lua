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

return {{
    Blocks = get_description,
    Meta = function (meta)
        meta.description = description
        meta.runes = string.format("%0.1f", runes/1000)
        meta.read_time = string.format("%0.1f", runes/400)
        return meta
    end,
    Image = function (img)
        img.attributes.loading = "lazy"
        return img
    end,
}}
