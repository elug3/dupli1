FROM nginx:1.27-alpine

COPY api/nginx.conf /etc/nginx/conf.d/default.conf
# Shared route table and upstream names, included by the config above.
COPY api/gateway/*.conf /etc/nginx/dupli1/

EXPOSE 80 443
