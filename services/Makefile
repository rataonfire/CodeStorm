.PHONY: up down logs clean migrate

up:
	docker-compose up -d

down:
	docker-compose down

logs:
	docker-compose logs -f

clean:
	docker-compose down -v
	rm -rf postgres_data redis_data

migrate:
	docker-compose exec postgres psql -U recon -d recon -f /docker-entrypoint-initdb.d/001_initial.sql