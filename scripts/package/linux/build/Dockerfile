FROM ubuntu:22.04
ARG version
RUN apt update && apt install -y ruby-dev build-essential squashfs-tools
RUN apt install -y rpm
RUN gem i fpm
CMD ["fpm", "--help"]
