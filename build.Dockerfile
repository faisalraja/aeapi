FROM gcr.io/gcpug-container/appengine-go:1.11-alpine

RUN apk add --no-cache python2

WORKDIR /app
COPY . .
COPY env.yaml.sample env.yaml

EXPOSE 8110
EXPOSE 8111
VOLUME ["/search"]
CMD ["dev_appserver.py", "/app/app.yaml", "--log_level=debug", "--search_indexes_path=/search/data", "--host=0.0.0.0", "--port=8110","--admin_host=0.0.0.0", "--admin_port=8111", "--enable_host_checking=false"]
