# Sistema de Temperatura por CEP com OpenTelemetry

Este projeto implementa um sistema distribuído em **Go** que consulta a temperatura atual de uma cidade a partir de um **CEP**. O sistema utiliza **OpenTelemetry** para *tracing* distribuído e **Zipkin** para visualização dos traces.

---

## Arquitetura

O sistema é composto por dois microserviços, um collector OpenTelemetry e o Zipkin:

- **Serviço A (8080)**: Responsável por receber e validar o CEP do usuário  
- **Serviço B (8081)**: Orquestra a consulta do CEP e busca da temperatura  
- **OpenTelemetry Collector**: Coleta e processa os traces gerados pelos serviços  
- **Zipkin**: Visualização dos traces para monitoramento e diagnóstico

---

## Tecnologias Utilizadas

- **Go 1.24**: Linguagem de programação  
- **OpenTelemetry**: Instrumentação para observabilidade  
- **Zipkin**: Visualização de traces  
- **Chi Router**: Roteamento HTTP  
- **Docker & Docker Compose**: Conteinerização e orquestração  

**APIs Externas:**  
- **ViaCEP**: Para consulta de informações do CEP  
- **WeatherAPI**: Para consulta de temperaturas

---

## Como Executar

### Pré-requisitos
- Docker  
- Docker Compose

### Instruções
1. Clone este repositório  
2. Execute o sistema completo com Docker Compose:
   ```bash
   docker-compose up --build
   ```

3. Acesse o Zipkin para visualizar os traces:  
   Abra no navegador: `http://localhost:9411`

---

## Como Usar a API

### Consultar Temperatura por CEP

**Endpoint:** `POST /cep`

**Request:**
```bash
curl -X POST http://localhost:8080/cep   -H "Content-Type: application/json"   -d '{"cep": "01001000"}'
```

**Resposta de Sucesso (200 OK):**
```json
{
  "city": "São Paulo",
  "temp_C": 28.5,
  "temp_F": 83.3,
  "temp_K": 301.5
}
```

### Erros Possíveis
- **422 Unprocessable Entity**: CEP inválido (não possui 8 dígitos numéricos)  
- **404 Not Found**: CEP não encontrado  
- **500 Internal Server Error**: Erro ao processar a requisição

---

## Observabilidade

### OpenTelemetry
O projeto utiliza **OpenTelemetry** para instrumentação de código, gerando *spans* e *traces* que permitem acompanhar a execução distribuída das requisições. Cada operação importante (como validação de CEP, consulta à API ViaCEP e consulta à API WeatherAPI) é instrumentada com *spans*.

### Zipkin
O **Zipkin** é utilizado para visualizar os *traces* gerados pelo sistema. Ele permite:
- Visualizar o tempo total de cada requisição  
- Identificar gargalos de performance  
- Acompanhar o fluxo de execução entre os serviços

Para acessar o Zipkin, abra `http://localhost:9411` em seu navegador após iniciar o sistema.

---

## Detalhes da Implementação

### Serviço A
- Recebe CEP via endpoint `POST /cep`  
- Valida se o CEP possui 8 dígitos numéricos  
- Repassa o CEP para o Serviço B  
- Propaga o contexto de *tracing* para o Serviço B

### Serviço B
- Recebe o CEP do Serviço A  
- Consulta a API ViaCEP para obter a cidade  
- Consulta a API WeatherAPI para obter a temperatura atual  
- Converte a temperatura para Celsius, Fahrenheit e Kelvin  
- Retorna os dados formatados

---

## Conversões de Temperatura

- **Celsius para Fahrenheit:**  
  \( F = C \times 1{,}8 + 32 \)

- **Celsius para Kelvin:**  
  \( K = C + 273 \)

