module github.com/sonatype-nexus-community/bbash

go 1.16

require (
	github.com/DATA-DOG/go-sqlmock v1.5.0
	github.com/golang-migrate/migrate/v4 v4.14.1
	github.com/joho/godotenv v1.3.0
	github.com/labstack/echo/v4 v4.2.2
	github.com/stretchr/testify v1.7.0
)

// fix: CVE-2021-3121 in pkg:golang/github.com/gogo/protobuf@1.3.1
replace github.com/dhui/dktest => github.com/dhui/dktest v0.3.4
