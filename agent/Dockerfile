FROM python:3.14-slim

ARG VERSION=dev
ARG VCS_REF=unknown
ARG BUILD_DATE=unknown

WORKDIR /app

RUN useradd --system --create-home --uid 10001 up2date \
    && chown -R up2date:up2date /app

COPY app/ /app/

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1

LABEL org.opencontainers.image.title="up2date-agent" \
      org.opencontainers.image.description="Docker-side collector that publishes retained MQTT node snapshots and status summaries" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${VCS_REF}" \
      org.opencontainers.image.created="${BUILD_DATE}"

USER 10001:10001

CMD ["python", "/app/up2date_agent.py"]
