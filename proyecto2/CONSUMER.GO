package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/go-redis/redis/v8"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/confluentinc/confluent-kafka-go/kafka"
)

const urlmongo = "mongodb://MongoDB:27017/"

// const urlmongo = "mongodb://localhost:27017/"
const topic = "myTopic"

const redisurl = "RedisDB:6379"

//const redisurl = "localhost:6379"

const urlkafka = "kafka:9092"

//const urlkafka = "localhost:9092"

var ctx = context.TODO()
var conexion = conectarMongo()
var redisdb = conectarRedis()

type Voto struct {
	Voto  string `bson:"voto"`
	Fecha string `bson:"fecha"`
}

type Data struct {
	Banda string `json:"banda"`
	Album string `json:"album"`
	Year  string `json:"year"`
	Rank  string `json:"rank"`
}

type votosR struct {
	Banda    string `json:"banda"`
	Album    string `json:"album"`
	Year     string `json:"year"`
	Rank     string `json:"rank"`
	Contador string `json:"contador"`
}

func conectarMongo() *mongo.Client {
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(urlmongo))
	if err != nil {
		log.Println("Error al conectar a MongoDB", err)
		return nil
	}
	fmt.Println("Conexión exitosa a MongoDB")
	return client
}

func conectarRedis() *redis.Client {
	client := redis.NewClient(&redis.Options{
		Addr:     redisurl,
		Password: "",
		DB:       0,
	})

	_, err := client.Ping(ctx).Result()
	if err != nil {
		log.Println("Error al conectar a Redis:", err)
		return nil
	}

	fmt.Println("Conexión exitosa a Redis")
	return client
}

func iniciarBase() *mongo.Collection {
	fmt.Println("Iniciando base de Mongo")
	base := conexion.Database("db1").Collection("votos")
	var primerDato Voto
	err := base.FindOne(ctx, bson.D{}).Decode(&primerDato)
	if err != nil {
		log.Println("Error al decodificar los datos", err)
	}

	result, err := base.DeleteMany(ctx, bson.M{"voto": bson.M{"$ne": primerDato.Voto}})

	if err != nil {
		log.Println("Error al eliminar los datos Mongo", err)
	}

	fmt.Println("Se eliminaron", result.DeletedCount, "registros de Mongo")
	return base
}

func insertarMongo(base *mongo.Collection, voto Voto) {

	_, err := base.InsertOne(ctx, voto)
	if err != nil {
		log.Println("Error al insertar dato en Mongo", err)
	}
	fmt.Println("Dato insertado en Mongo")
}

func insertarRedis(jsonbotos votosR) error {
	err := redisdb.HIncrBy(context.Background(), "votos_banda", jsonbotos.Contador, 1).Err()
	if err != nil {
		log.Println("Error al incrementar votos_banda:", err)
		return err
	}
	fmt.Println("Registrado en Redis:", jsonbotos)
	return nil
}

func main() {

	c, err := kafka.NewConsumer(&kafka.ConfigMap{
		"bootstrap.servers": urlkafka,
		"group.id":          "myGroup",
		"auto.offset.reset": "earliest",
	})

	if err != nil {
		log.Printf("Error al crear consumer: %s", err)
	}
	defer c.Close()

	c.SubscribeTopics([]string{topic}, nil)

	base := iniciarBase()

	go func() {
		for {
			msg, err := c.ReadMessage(time.Second)
			if err == nil {
				var data Data
				err := json.Unmarshal(msg.Value, &data)
				if err != nil {
					log.Printf("Error al convertir a json: %s", err)
					continue
				}

				fmt.Printf("Message on %s: %s\n", msg.TopicPartition, string(msg.Value))

				if data.Banda == "" || data.Album == "" || data.Year == "" || data.Rank == "" {
					log.Println("Error al obtener datos de la API datos vacios")
					continue

				}
				jsonbotos := votosR{
					Banda:    data.Banda,
					Album:    data.Album,
					Year:     data.Year,
					Rank:     data.Rank,
					Contador: fmt.Sprintf("%s_%s_%s_%s", data.Banda, data.Album, data.Year, data.Rank),
				}

				err = insertarRedis(jsonbotos)
				if err != nil {
					continue
				}

				votoobtenido := fmt.Sprintf("Banda: %s, Album: %s,Year:%s,Rank:%s", data.Banda, data.Album, data.Year, data.Rank)

				nuevoVoto := Voto{
					Voto:  votoobtenido,
					Fecha: time.Now().Format("02/01/2006, 15:04:05"),
				}

				insertarMongo(base, nuevoVoto)

			} else {
				fmt.Printf("Consumer error: %v (%v)\n", err, msg)
			}

			time.Sleep(100 * time.Millisecond)
		}

	}()
	select {}

}