mig-up:
	goose up

mig-down:
	goose down

mig-reset:
	goose reset

mig-status:
	goose status

create-migrate:
	goose create $(n) sql

sql-gen:
	sqlc generate

doc-gen:
	swag init -g cmd/main.go

run-server:
	go run cmd/main.go
