module github.com/elug3/dupli1/support

go 1.26.3

require (
	github.com/elug3/dupli1/shared v0.0.0-00010101000000-000000000000
	github.com/oklog/ulid/v2 v2.1.0
)

require github.com/lib/pq v1.12.3

replace github.com/elug3/dupli1/shared => ../shared
