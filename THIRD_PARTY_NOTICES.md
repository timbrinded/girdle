# Third-party notices

Girdle's original code and SVG assets are licensed under [Apache-2.0](LICENSE). The benchmark includes patches and tests derived from the projects below. Their original copyright and license notices are retained here; those materials remain under their upstream licenses.

The task's `source` file records its upstream URL and starting commit. Full upstream repositories are cloned into an ignored local cache when tasks are prepared. The notice files below are copied from the pinned upstream revisions, rather than reconstructed summaries.

| Project | Benchmark prefixes | Upstream license notice |
| --- | --- | --- |
| [BurntSushi/toml](https://github.com/BurntSushi/toml) | `tm-` | [MIT](docs/licenses/BurntSushi-toml.txt) ([pinned source](https://github.com/BurntSushi/toml/blob/e954445330492727c18483838a15f88a4fc2d03a/COPYING)) |
| [Masterminds/semver](https://github.com/Masterminds/semver) | `sv-` | [MIT](docs/licenses/Masterminds-semver.txt) ([pinned source](https://github.com/Masterminds/semver/blob/363e594e96d8cb75297266983f3905497b778d4e/LICENSE.txt)) |
| [dustin/go-humanize](https://github.com/dustin/go-humanize) | `hz-` | [MIT notice](docs/licenses/dustin-go-humanize.txt) ([pinned source](https://github.com/dustin/go-humanize/blob/71f653b2f435ca3898cffe7097697aa59227a87e/LICENSE)) |
| [expr-lang/expr](https://github.com/expr-lang/expr) | `ex-` | [MIT](docs/licenses/expr-lang-expr.txt) ([pinned source](https://github.com/expr-lang/expr/blob/44dfabc57310d2ab9a6dd7a4799039bfc50df556/LICENSE)), [988663ce4e70 notice](docs/licenses/expr-lang-expr-988663ce4e70.txt) |
| [gobwas/glob](https://github.com/gobwas/glob) | `gb-` | [MIT](docs/licenses/gobwas-glob.txt) ([pinned source](https://github.com/gobwas/glob/blob/e9b2193e74ebd31d5f1cc6ea306a44e6a7995bc2/LICENSE)) |
| [google/go-cmp](https://github.com/google/go-cmp) | `cmp-` | [BSD-3-Clause](docs/licenses/google-go-cmp.txt) ([pinned source](https://github.com/google/go-cmp/blob/c3ad8435e7bef96af35732bc0789e5a2278c6d5f/LICENSE)) |
| [gorilla/mux](https://github.com/gorilla/mux) | `mux-` | [BSD-3-Clause](docs/licenses/gorilla-mux.txt) ([pinned source](https://github.com/gorilla/mux/blob/e44017df2b8798f6bfff81fff1c0b319c1a54496/LICENSE)) |
| [hashicorp/go-version](https://github.com/hashicorp/go-version) | `gv-` | [MPL-2.0](docs/licenses/hashicorp-go-version.txt) ([pinned source](https://github.com/hashicorp/go-version/blob/c2de2312d363381e4254abec260c6df2b1226483/LICENSE)) |
| [more-itertools/more-itertools](https://github.com/more-itertools/more-itertools) | `mi-` | [MIT](docs/licenses/more-itertools-more-itertools.txt) ([pinned source](https://github.com/more-itertools/more-itertools/blob/9210d54527ddfa63ebe75cd5b5daa0201902c674/LICENSE)) |
| [pelletier/go-toml](https://github.com/pelletier/go-toml) | `gt-` | [MIT notice](docs/licenses/pelletier-go-toml.txt) ([pinned source](https://github.com/pelletier/go-toml/blob/2edc61f17159e4fde0f26098841160013015430b/LICENSE)) |
| [spf13/pflag](https://github.com/spf13/pflag) | `pf-` | [BSD-3-Clause](docs/licenses/spf13-pflag.txt) ([pinned source](https://github.com/spf13/pflag/blob/d5e0c0615acee7028e1e2740a11102313be88de1/LICENSE)) |
| [tidwall/gjson](https://github.com/tidwall/gjson) | `gj-` | [MIT](docs/licenses/tidwall-gjson.txt) ([pinned source](https://github.com/tidwall/gjson/blob/7cadbb575617824cfa39a8a7e1cf0b059a7c6f67/LICENSE)) |
| [yuin/goldmark](https://github.com/yuin/goldmark) | `gm-` | [MIT](docs/licenses/yuin-goldmark.txt) ([pinned source](https://github.com/yuin/goldmark/blob/d44652d1743e4fb4cf7b5cb560d7480692ea76c9/LICENSE)) |

Runtime dependencies are declared in [go.mod](go.mod) and [go.sum](go.sum); their upstream distributions carry their own licenses.
