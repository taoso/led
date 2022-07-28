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
        meta.description = description
        return meta
    end
}}
