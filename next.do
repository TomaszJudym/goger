Check 
https://embed.gog.com/games/ajax/filtered?mediaType=game&search=Witcher%203
Or generally where to take game IDs from
Then reviews can be accessed like:
https://reviews.gog.com/v1/products/1256837418/reviews?limit=2&page=13

game id is just in response document undeR:
product_id: 
Can already grep for it from golang

Games actually also can be scrapped with go
to not make 2GB container.

Just via:
https://catalog.gog.com/v1/catalog?limit=48&order=desc%3Atrending&productType=in%3Agame%2Cpack%2Cdlc%2Cextras&page=2&countryCode=PL&locale=en-US&currencyCode=PLN
To take page 2.
Inspected with network at /games

Working nice, now select if save it in scylla, surreal or postgres.
Postgres cool but need flattening
Surreal edgy
scylla insteresting and more mature than surreal

11.11.2023

1. https://reviews.gog.com/v1/products/1256837418/reviews?limit=2&page=13
^ Iterate over it to figure out all games.

2. Then take one after another, go to its page and take reviews.

3. Save it in db, check what frequency will be periodically working
to not get blocked by API.

4. On 429 TooManyRequests wait for few min to let page calm down.
Do it incrementally. 1min, 2, 4, 8 , 16 and so on.

##########################################

Add configurable batch size.
By default make it 500.
On error backoff and try to fetch less up to as little as 10.

For some reason {formatter} in urls works with:
product_card_v2_mobile_slider_639
Found on cyberpunk phantom liberty store page.

##########################################

- UI:
  Add sorting buttons per ID, title and Reviews count.
  Add Search by title. Check if postgres have text search possible.
  With stuff like mispelling etc.


- Redis: to cache responses

- Just fix UI
