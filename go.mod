module github.com/sonatype-nexus-community/bbash

go 1.16

require (
	github.com/golang-migrate/migrate/v4 v4.14.1
	github.com/joho/godotenv v1.3.0
	github.com/labstack/echo/v4 v4.2.2
)

// fix: CVE-2021-3121 in pkg:golang/github.com/gogo/protobuf@1.3.1
replace github.com/dhui/dktest => github.com/dhui/dktest v0.3.4
