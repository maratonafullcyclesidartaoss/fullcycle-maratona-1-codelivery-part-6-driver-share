# Maratona Full Cycle - Codelivery - Part V - Driver - CI/CD

O projeto consiste em:

- Um sistema de monitoramento de veículos de entrega em tempo real.

Requisitos:

- Uma transportadora quer fazer o agendamento de suas entregas;
- Ela também quer ter o _feedback_ instantâneo de quando a entrega é realizada;
- Caso haja necessidade de acompanhar a entrega com mais detalhes, o sistema deverá informar, em tempo real, a localização do motorista no mapa.

#### Que problemas de negócio o projeto poderia resolver?

- O projeto pode ser adaptado para casos de uso onde é necessário rastrear e monitorar carros, caminhões, frotas e remessas em tempo real, como na logística e na indústria automotiva.

Dinâmica do sistema:

1. A aplicação _Order_ (_React_/_Nest.js_) é responsável pelas ordens de serviço (ou pedidos) e vai conter a tela de agendamento de pedidos de entrega. A criação de uma nova ordem de serviço começa o processo para que o motorista entregue a mercadoria;

2. A aplicação _Driver_ (_Go_) é responsável por gerenciar o contexto limitado de motoristas. Neste caso, sua responsabilidade consiste em disponibilizar os _endpoints_ de consulta;

3. Para a criação de uma nova ordem de serviço, a aplicação _Order_ obtém de _Driver_ os dados dos motoristas. Neste caso, _REST_ é uma opção pertinente, porque a comunicação deve ser a mais simples possível;

4. Após criar a nova ordem de serviço, _Order_ notifica a aplicação _Mapping_ (_Nest.js_/_React_) via _RabbitMQ_ de que o motorista deve iniciar a entrega. _Mapping_ é a aplicação que vai exibir no mapa a posição do motorista em tempo real. A aplicação _Simulator_ (_Go_) também é notificada sobre o início da entrega e começa a enviar para a aplicação _Mapping_ as posições do veículo;

5. Ao finalizar a entrega, a aplicação _Mapping_ notifica via _RabbitMQ_ a aplicação _Order_ de que o produto foi entregue e a aplicação altera o _status_ da entrega de Pendente para Entregue.

## Tecnologias

#### Operate What You Build

Nesta sexta versão, trabalhamos com tecnologias relacionadas aos processos de monitoramento e observabilidade, aplicados a um _API Gateway_ operando com a _API_ da aplicação _Driver_.

- Backend

  - Golang

- API Gateway

  - Kong

- Monitoramento

  - Prometheus
  - EFK
    - Elasticsearch
    - Fluentd
    - Kibana

- GitOps Tool

  - ArgoCD

- Deploy

  - Kubernetes GKE

### O que faremos

### Destruindo a infraestrutura

Chegou o momento de liberar os recursos no ambiente _cloud_.

Para tanto, iremos utilizar o _Terraform_, que irá remover todos os recursos relacionados com o _cluster GKE_.

```
cd terraform/

terraform destroy
```

![Cluster GKE destruído](./images/cluster-gke-destruido.png)

#### Referências

UDEMY. Como implementar GitFlow en Gitlab y Github. 2023. Disponível em: <https://www.udemy.com/course/como-implementar-gitflow-en-gitlab-y-github>. Acesso em: 26 mai. 2023.

FULL CYCLE 3.0. Integração contínua. 2023. Disponível em: <https://plataforma.fullcycle.com.br>. Acesso em: 26 mai. 2023.

FULL CYCLE 3.0. Padrões e técnicas avançadas com Git e Github. 2023. Disponível em: <https://plataforma.fullcycle.com.br>. Acesso em: 26 mai. 2023.

SONARCLOUD. Clean code in your cloud workflow with {SonarCloud}. 2023. Disponível em: <https://www.sonarsource.com/products/sonarcloud>. Acesso em: 26 mai. 2023.

FULL CYCLE 3.0. API Gateway com Kong e Kubernetes. 2023. Disponível em: <https://plataforma.fullcycle.com.br>. Acesso em: 31 mai. 2023.

SPECTRAL. Create a Ruleset. 2023. Disponível em: <https://meta.stoplight.io/docs/spectral/01baf06bdd05a-create-a-ruleset>. Acesso em: 01 jun. 2023.

TERRAFORM. Provision a GKE Cluster (Google Cloud). 2023. Disponível em: <https://developer.hashicorp.com/terraform/tutorials/kubernetes/gke>. Acesso em: 01 jun. 2023.

FULL CYCLE 3.0. GitOps. 2023. Disponível em: <https://plataforma.fullcycle.com.br>. Acesso em: 02 jun. 2023.

ARGO CD. Getting Started. 2023. Disponível em: <https://argo-cd.readthedocs.io/en/stable/getting_started>. Acesso em: 02 jun. 2023.

FULL CYCLE 3.0. Terraform. 2023. Disponível em: <https://plataforma.fullcycle.com.br>. Acesso em: 04 jun. 2023.

KUBERNETES DOCUMENTATION. Declarative Management of Kubernetes Objects Using Kustomize. 2023. Disponível em: <https://kubernetes.io/docs/tasks/manage-kubernetes-objects/kustomization>. Acesso em: 04 jun. 2023.
