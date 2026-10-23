# Vendored mauview

Fork of [go.mau.fi/mauview](https://github.com/tulir/mauview) v0.2.1, used via a `replace` directive in gomuks' `go.mod`.

Changes from upstream:

* `Application.SetShowHandler` lets the caller replace the final `screen.Show()` of each redraw,
  which gomuks uses to write sixel images to the terminal after tcell has flushed its cells.
