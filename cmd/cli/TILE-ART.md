# CLI 牌面字符规约

## 主题

- `unicode`：默认主题，4 cell × 4 行，中文牌面加 Unicode 框线。
- `ascii`：降级主题，4 cell × 4 行，使用 `+--+` 与协议短码。
- `emoji`：麻将 emoji 主题，4 cell × 4 行，中间一行使用 U+1F000 到 U+1F02B。

## 横向牌面

```text
--+   ┌──┐   🀇
|m1|   │一│
|  |   │万│
+--+   └──┘
```

所有主题都必须保持 `TileArtWidth=4` 与 `TileArtHeight=4`，避免切换主题时重排版。

## 竖向压缩牌面

`RenderTileVertical` 输出 2 cell × 3 行，仅用于侧栏或窄区域。主手牌仍使用横向牌面。

## 徽章

```text
unicode / emoji: 庄=⊕  就绪=✓  自己=★  托管=⏸  机器人=◇BOT
ascii:           庄=[Z] 就绪=[V] 自己=[*] 托管=[P] 机器人=[B]
```

emoji 主题依赖终端字体支持麻将牌符号；若出现豆腐块，请切回 `unicode` 或 `ascii`。
