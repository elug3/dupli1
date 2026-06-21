FROM nginx:alpine
COPY nginx.conf /etc/nginx/nginx.conf
COPY nginx-alb.conf /etc/nginx/nginx-alb.conf
# Local/docker-compose: mount ./certs and use nginx.conf (TLS on :443).
# ECS/ALB: set NGINX_CONF=/etc/nginx/nginx-alb.conf (HTTP on :80 behind ALB).
