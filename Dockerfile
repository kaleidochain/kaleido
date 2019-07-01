# Build in a stock Go builder container
FROM kaleidochain/builder as builder

ADD . /kaleido
RUN cd /kaleido && make clean && make clean-deplibs && make deplibs && make kalgo

# Pull into a second stage deploy alpine container
FROM alpine:latest

RUN apk add --no-cache ca-certificates libstdc++
COPY --from=builder /kaleido/build/bin/kalgo /usr/local/bin/

EXPOSE 8545 8546 38883 38883/udp
ENTRYPOINT ["kalgo"]
