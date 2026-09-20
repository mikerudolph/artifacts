# Repository rules

Never write prose comments in code, including implementation files, tests, examples, and documentation-site source. Express intent through clear names and structure; put explanations in Markdown documentation when needed.

Retain only machine-interpreted directives required by the compiler, build system, or static analysis, such as `//go:embed` and `//nolint:gosec`. Do not append prose explanations to these directives.
