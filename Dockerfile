FROM golang:1.22
WORKDIR /app/
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a -o offload -v main.go
RUN mkdir empty

FROM scratch
COPY --from=0 /app/offload /offload
COPY --from=0 /app/assets /assets
COPY --from=0 /app/templates /templates
COPY --from=0 /app/empty /lists
COPY --from=subtlepseudonym/healthcheck:0.1.1 /healthcheck /healthcheck

EXPOSE 9494/tcp
HEALTHCHECK --interval=60s --timeout=2s --retries=3 --start-period=2s \
	CMD ["/healthcheck", "localhost:9494", "/ok"]

CMD ["/offload"]
