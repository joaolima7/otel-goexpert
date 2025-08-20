# Sistema de Temperatura por CEP com OpenTelemetry

Este projeto implementa um sistema distribuído em Go que consulta a temperatura atual de uma cidade a partir de um CEP, utilizando OpenTelemetry para tracing distribuído e Zipkin para visualização.

## Arquitetura

- **Serviço A**: Recebe e valida o CEP
- **Serviço B**: Consulta o CEP e a temperatura
- **OpenTelemetry Collector**: Coleta e processa os traces
- **Zipkin**: Visualização dos traces

## Requisitos

- Docker
- Docker Compose

## Como executar

1. Clone este repositório
2. Execute o sistema completo com Docker Compose:

```bash
docker-compose up -d
```