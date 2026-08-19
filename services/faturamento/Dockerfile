# Multi-stage: o SDK (~800MB) fica no estagio de construcao. A imagem final
# carrega apenas o runtime do ASP.NET e os binarios publicados.

FROM mcr.microsoft.com/dotnet/sdk:10.0-alpine AS construcao

WORKDIR /src

# O csproj sozinho primeiro: enquanto as dependencias nao mudam, o restore vem
# do cache de camadas em vez de rebaixar todos os pacotes NuGet.
COPY Faturamento.Api/Faturamento.Api.csproj Faturamento.Api/
RUN dotnet restore Faturamento.Api/Faturamento.Api.csproj

COPY Faturamento.Api/ Faturamento.Api/
RUN dotnet publish Faturamento.Api/Faturamento.Api.csproj \
      -c Release -o /app --no-restore

FROM mcr.microsoft.com/dotnet/aspnet:10.0-alpine

WORKDIR /app
COPY --from=construcao /app .

# APP_UID vem definido nas imagens oficiais: processo sem privilegios de root.
USER $APP_UID
EXPOSE 8082

ENTRYPOINT ["dotnet", "Faturamento.Api.dll"]
