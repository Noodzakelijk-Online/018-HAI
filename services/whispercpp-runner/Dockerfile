FROM debian:bookworm-slim AS build

ARG WHISPER_CPP_REF=v1.9.1

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates cmake g++ git make \
    && rm -rf /var/lib/apt/lists/*

RUN git clone --depth 1 --branch "${WHISPER_CPP_REF}" https://github.com/ggml-org/whisper.cpp.git /src/whisper.cpp \
    && cmake -S /src/whisper.cpp -B /src/whisper.cpp/build -DCMAKE_BUILD_TYPE=Release -DWHISPER_BUILD_TESTS=OFF \
    && cmake --build /src/whisper.cpp/build --config Release --target whisper-cli -j2

FROM debian:bookworm-slim

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates ffmpeg python3 \
    && rm -rf /var/lib/apt/lists/* \
    && useradd --system --uid 65532 --create-home hai

COPY --from=build /src/whisper.cpp/build/bin/ /usr/local/lib/whispercpp/
RUN ln -s /usr/local/lib/whispercpp/whisper-cli /usr/local/bin/whisper-cli
COPY app.py /app/app.py

ENV LD_LIBRARY_PATH=/usr/local/lib/whispercpp
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["python3", "/app/app.py"]
