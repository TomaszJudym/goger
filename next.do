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
scylla insteresting and more mature than surreal

11.11.2023

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
  -> Turn into postgresql UNLOGGED table

- Make plan what to display finally.
1. Per game - list all languages
2. 5 most popular words per language.
3. Rating per language.
4. Avg review length per language.
5. 5 most popular colors in images.
6. Developer, numbers of games, % of games
7. % of games per developer
8. On each game side - chart time to number of reviews
9. Store thumbnails for games and display them.

NOW: - finish main page which is showing general statistics
and only below list of games with most reviews.
