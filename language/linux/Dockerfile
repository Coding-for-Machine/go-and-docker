# Alpine Linux'ni yuklaymiz
FROM alpine:latest

# Kerakli paketlarni o‘rnatamiz
RUN apk add --no-cache bash curl tar && \
    curl -L -o gotty_linux_amd64.tar.gz https://github.com/sorenisanerd/gotty/releases/download/v1.0.1/gotty_linux_amd64.tar.gz && \
    ls -lh gotty_linux_amd64.tar.gz && \
    file gotty_linux_amd64.tar.gz && \
    tar -xvzf gotty_linux_amd64.tar.gz || cat gotty_linux_amd64.tar.gz && \
    mv gotty /usr/local/bin/gotty && \
    chmod +x /usr/local/bin/gotty

# Web terminalni ishga tushiramiz
CMD ["/usr/local/bin/gotty", "-w", "/bin/bash"]

# Portni ochamiz
EXPOSE 8080
